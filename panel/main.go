// aios-panel — AI Engineer OS 的本機控制台（零外部依賴，只綁 127.0.0.1）。
//
// 設計原則：panel 只是「協定檔的讀者與寫者」，判斷力留在 agent——
//   讀：supervisor lock（幾個 agent 在跑）、doing/backlog/done、checkpoint、
//       PAUSED、last_run、receipts frontmatter、ai/queue 領先數
//   寫：只寫兩種協定檔——PAUSED 的「## 人類回覆」節、.ai/STOP 的建立/刪除
//   出貨（push）是對外動作，panel 只顯示可出貨數量與 /ai-ship 指令。
//
// 用法：
//   go run ./panel -repos /path/a,/path/b        （或編譯後 aios-panel）
//   沒給 -repos 時讀 ~/.aios-repos（一行一個 repo 路徑，# 開頭為註解）
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type RepoState struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Missing         bool     `json:"missing"` // .ai/ 不存在
	SupervisorAlive bool     `json:"supervisor_alive"`
	SupervisorPID   int      `json:"supervisor_pid,omitempty"`
	Stopped         bool     `json:"stopped"` // .ai/STOP 存在
	Phase           string   `json:"phase"`
	Iteration       int      `json:"iteration"`
	CurrentTask     string   `json:"current_task"` // doing.yaml 的 id+title
	Backlog         []string `json:"backlog"`      // 前 5 筆 "T-NNN title"
	BacklogCount    int      `json:"backlog_count"`
	DoneCount       int      `json:"done_count"`
	Paused          bool     `json:"paused"`
	PausedQuestion  string   `json:"paused_question,omitempty"`
	PausedAnswered  bool     `json:"paused_answered"`
	Shippable       int      `json:"shippable"`   // ai/queue 領先主分支的 commit 數
	DirtyCount      int      `json:"dirty_count"` // working tree 未 commit 的檔案數（未記帳警訊）
	LastRunStatus   string   `json:"last_run_status,omitempty"`
	LastRunCost     string   `json:"last_run_cost,omitempty"`
	LastRunAt       string   `json:"last_run_at,omitempty"`
	Receipts        []string `json:"receipts"` // 最近 3 張 "日期/NNN [status] [human]? title"
}

func main() {
	addr := flag.String("addr", "127.0.0.1:7777", "監聽位址（僅限本機）")
	reposFlag := flag.String("repos", "", "逗號分隔的 repo 路徑；空 = 讀 ~/.aios-repos")
	flag.Parse()

	repos := loadRepos(*reposFlag)
	if len(repos) == 0 {
		fmt.Fprintln(os.Stderr, "沒有 repo：用 -repos /a,/b 或在 ~/.aios-repos 一行一個路徑")
		os.Exit(64)
	}
	if !strings.HasPrefix(*addr, "127.0.0.1:") && !strings.HasPrefix(*addr, "localhost:") {
		fmt.Fprintln(os.Stderr, "拒絕綁定非 localhost 位址（panel 無認證，僅供本機）")
		os.Exit(64)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})
	http.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		states := make([]RepoState, 0, len(repos))
		for _, p := range repos {
			states = append(states, readRepo(p))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(states)
	})
	http.HandleFunc("/api/answer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		repo, text := r.FormValue("repo"), strings.TrimSpace(r.FormValue("text"))
		if !allowed(repos, repo) || text == "" {
			http.Error(w, "unknown repo or empty answer", 400)
			return
		}
		paused := filepath.Join(repo, ".ai", "PAUSED")
		if _, err := os.Stat(paused); err != nil {
			http.Error(w, "此 repo 沒有待回答的 PAUSED", 409)
			return
		}
		f, err := os.OpenFile(paused, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "\n## 人類回覆（%s）\n%s\n", time.Now().Format("2006-01-02 15:04"), text)
		w.Write([]byte("ok"))
	})
	http.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		repo, action := r.FormValue("repo"), r.FormValue("action")
		if !allowed(repos, repo) {
			http.Error(w, "unknown repo", 400)
			return
		}
		stop := filepath.Join(repo, ".ai", "STOP")
		switch action {
		case "stop":
			if err := os.WriteFile(stop, []byte("panel 於 "+time.Now().Format(time.RFC3339)+" 要求停止\n"), 0o644); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		case "resume":
			// STOP 是信號旗（同 PAUSED），移除即恢復——非審計紀錄
			if err := os.Remove(stop); err != nil && !os.IsNotExist(err) {
				http.Error(w, err.Error(), 500)
				return
			}
		default:
			http.Error(w, "action 必須是 stop|resume", 400)
			return
		}
		w.Write([]byte("ok"))
	})

	fmt.Printf("aios-panel: http://%s  （repos: %d 個）\n", *addr, len(repos))
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func loadRepos(flagVal string) []string {
	var raw []string
	if flagVal != "" {
		raw = strings.Split(flagVal, ",")
	} else if home, err := os.UserHomeDir(); err == nil {
		if b, err := os.ReadFile(filepath.Join(home, ".aios-repos")); err == nil {
			raw = strings.Split(string(b), "\n")
		}
	}
	var out []string
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r != "" && !strings.HasPrefix(r, "#") {
			out = append(out, filepath.Clean(r))
		}
	}
	return out
}

func allowed(repos []string, p string) bool {
	for _, r := range repos {
		if r == filepath.Clean(p) {
			return true
		}
	}
	return false
}

// ---------- 讀取協定檔（容錯優先：壞檔回空值，不 panic） ----------

func readRepo(path string) RepoState {
	s := RepoState{Name: filepath.Base(path), Path: path}
	ai := filepath.Join(path, ".ai")
	if _, err := os.Stat(ai); err != nil {
		s.Missing = true
		return s
	}
	// supervisor lock
	if b, err := os.ReadFile(filepath.Join(ai, "supervisor", "lock")); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			if syscall.Kill(pid, 0) == nil {
				s.SupervisorAlive, s.SupervisorPID = true, pid
			}
		}
	}
	_, err := os.Stat(filepath.Join(ai, "STOP"))
	s.Stopped = err == nil
	// checkpoint
	if m := readJSON(filepath.Join(ai, "state", "checkpoint.json")); m != nil {
		s.Phase, _ = m["phase"].(string)
		if f, ok := m["iteration"].(float64); ok {
			s.Iteration = int(f)
		}
	}
	// tasks
	s.CurrentTask = firstTask(filepath.Join(ai, "tasks", "doing.yaml"))
	s.Backlog, s.BacklogCount = taskList(filepath.Join(ai, "tasks", "backlog.yaml"), 5)
	_, s.DoneCount = taskList(filepath.Join(ai, "tasks", "done.yaml"), 0)
	// PAUSED
	if b, err := os.ReadFile(filepath.Join(ai, "PAUSED")); err == nil {
		s.Paused = true
		content := string(b)
		s.PausedAnswered = strings.Contains(content, "## 人類回覆")
		q := content
		if i := strings.Index(content, "## 人類回覆"); i >= 0 {
			q = content[:i]
		}
		if len(q) > 400 {
			q = q[:400] + "…"
		}
		s.PausedQuestion = strings.TrimSpace(q)
	}
	// last_run
	if m := readJSON(filepath.Join(ai, "supervisor", "last_run.json")); m != nil {
		s.LastRunStatus, _ = m["last_status"].(string)
		s.LastRunAt, _ = m["at"].(string)
		if f, ok := m["total_cost_usd"].(float64); ok {
			s.LastRunCost = strconv.FormatFloat(f, 'f', 2, 64)
		}
	}
	// shippable：ai/queue 領先主分支多少 commit（主分支猜 main，退 master）
	for _, base := range []string{"main", "master"} {
		out, err := exec.Command("git", "-C", path, "rev-list", "--count", base+"..ai/queue").Output()
		if err == nil {
			s.Shippable, _ = strconv.Atoi(strings.TrimSpace(string(out)))
			break
		}
	}
	// dirty：working tree 有未 commit 的變更 = 有工作還沒被 /ai-wrap 記帳
	if out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output(); err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			s.DirtyCount = len(strings.Split(t, "\n"))
		}
	}
	// receipts（最近 3）
	s.Receipts = recentReceipts(filepath.Join(ai, "receipts"), 3)
	return s
}

func readJSON(p string) map[string]any {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// taskList 讀 tasks yaml：回傳前 n 筆 "id title" 與總數（行掃描，容錯）。
func taskList(p string, n int) ([]string, int) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, 0
	}
	var out []string
	var id string
	count := 0
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- id:") {
			count++
			id = strings.TrimSpace(strings.TrimPrefix(t, "- id:"))
		} else if strings.HasPrefix(t, "title:") && id != "" {
			if n == 0 || len(out) < n {
				title := strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "title:")), `"`)
				out = append(out, id+" "+title)
			}
			id = ""
		}
	}
	return out, count
}

func firstTask(p string) string {
	list, _ := taskList(p, 1)
	if len(list) > 0 {
		return list[0]
	}
	return ""
}

func recentReceipts(dir string, n int) []string {
	var files []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	// 檔名含日期與流水號，字典序 = 時間序
	for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
		files[i], files[j] = files[j], files[i]
	}
	if len(files) > n {
		files = files[:n]
	}
	var out []string
	for _, f := range files {
		b, _ := os.ReadFile(f)
		get := func(key string) string {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, key+":") {
					return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, key+":")), `"`)
				}
			}
			return ""
		}
		rel := filepath.Base(filepath.Dir(f)) + "/" + strings.TrimSuffix(filepath.Base(f), ".md")
		human := ""
		if get("source") == "human-interactive" {
			human = "[human] "
		}
		out = append(out, fmt.Sprintf("%s [%s] %s%s", rel, get("status"), human, get("title")))
	}
	return out
}
