// aios-panel — AI Engineer OS 的本機控制台（零外部依賴，只綁 127.0.0.1）。
//
// 設計原則：panel 只是「協定檔的讀者與寫者」，判斷力留在 agent——
//   讀：supervisor lock（幾個 agent 在跑）、doing/backlog/done、checkpoint、
//       PAUSED、last_run、receipts frontmatter、ai/queue 領先數
//   寫：只寫兩種協定檔——PAUSED 的「## 人類回覆」節、.ai/STOP 的建立/刪除
//   出貨（push）是對外動作，panel 只顯示可出貨數量與 /ai-ship 指令。
//   唯一例外：claude 帳號用量（/api/usage）——帳號層級、不是協定檔，
//   查一次要 spawn `claude -p "/usage"`（非零成本，~0.5s），所以刻意
//   跟 5 秒的 state 輪詢分開：60 秒快取一次，且只查一次（不分 repo）。
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
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	DashboardReady  bool     `json:"dashboard_ready"` // 卡片要不要顯示「儀表板」連結
	DevURL          string   `json:"dev_url,omitempty"` // ~/.aios-repos 該行第二欄（本機 dev server 網址，可選）
}

// dashboardScriptPath：supervisor/dashboard.sh 的路徑（-dashboard-script 設定）。
// 空字串 = 不重算，/dashboard 只讀既有的 .ai/reports/dashboard.html（若存在）。
var dashboardScriptPath string

func main() {
	addr := flag.String("addr", "127.0.0.1:7777", "監聽位址（僅限本機）")
	reposFlag := flag.String("repos", "", "逗號分隔的 repo 路徑；空 = 讀 ~/.aios-repos")
	dashboardScript := flag.String("dashboard-script", "", "supervisor/dashboard.sh 的絕對路徑；設定後點卡片上的儀表板連結會先重算（1 分鐘內的快照直接沿用），不設就只讀既有的 .ai/reports/dashboard.html")
	flag.Parse()
	dashboardScriptPath = *dashboardScript

	if len(loadRepos(*reposFlag)) == 0 {
		fmt.Fprintln(os.Stderr, "沒有 repo：用 -repos /a,/b 或在 ~/.aios-repos 一行一個路徑")
		os.Exit(64)
	}
	// repo 清單每個請求重讀（熱重載）：/ai-init 註冊新 repo 進 ~/.aios-repos
	// 後，5 秒內卡片自動出現，panel 不用重啟。-repos flag 給定時清單固定，
	// 重讀只是重切字串，成本可忽略。
	currentRepos := func() []string { return loadRepos(*reposFlag) }
	if !strings.HasPrefix(*addr, "127.0.0.1:") && !strings.HasPrefix(*addr, "localhost:") {
		fmt.Fprintln(os.Stderr, "拒絕綁定非 localhost 位址（panel 無認證，僅供本機）")
		os.Exit(64)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})
	http.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		repos := currentRepos()
		devURLs := loadDevURLs(*reposFlag)
		states := make([]RepoState, 0, len(repos))
		for _, p := range repos {
			states = append(states, readRepo(p, devURLs[p]))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(states)
	})
	http.HandleFunc("/api/usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(getUsage(currentRepos()))
	})
	http.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		repo := r.URL.Query().Get("repo")
		if !allowed(currentRepos(), repo) {
			http.Error(w, "unknown repo", 400)
			return
		}
		out := filepath.Join(repo, ".ai", "reports", "dashboard.html")
		if dashboardScriptPath != "" {
			stale := true
			if info, err := os.Stat(out); err == nil && time.Since(info.ModTime()) < 60*time.Second {
				stale = false
			}
			if stale {
				cmd := exec.Command(dashboardScriptPath, "--repo", repo)
				if err := cmd.Run(); err != nil {
					if _, statErr := os.Stat(out); statErr != nil {
						http.Error(w, "dashboard.sh 執行失敗且無舊快照可用："+err.Error(), 500)
						return
					}
					// 重算失敗但有舊檔——照樣送出舊快照，不擋畫面
				}
			}
		}
		if _, err := os.Stat(out); err != nil {
			http.Error(w, "尚未產生 dashboard.html——手動跑一次：\nsupervisor/dashboard.sh --repo "+repo, 404)
			return
		}
		http.ServeFile(w, r, out)
	})
	http.HandleFunc("/api/answer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		repo, text := r.FormValue("repo"), strings.TrimSpace(r.FormValue("text"))
		if !allowed(currentRepos(), repo) || text == "" {
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
		if !allowed(currentRepos(), repo) {
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

	fmt.Printf("aios-panel: http://%s  （repos: %d 個，清單熱重載）\n", *addr, len(currentRepos()))
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
			out = append(out, filepath.Clean(strings.Fields(r)[0]))
		}
	}
	return out
}

// loadDevURLs 讀 ~/.aios-repos 每行的第二欄（空白分隔，可選）：本機 dev
// server 網址，純人工維護，agent 不讀不寫、不是協定檔。格式：
// `{path} {url}`，例如 `/repo/a http://localhost:5173`。
// -repos flag 給的清單不支援這欄（CLI 用法維持簡單，回傳空 map）。
func loadDevURLs(flagVal string) map[string]string {
	out := map[string]string{}
	if flagVal != "" {
		return out
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	b, err := os.ReadFile(filepath.Join(home, ".aios-repos"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 1 {
			out[filepath.Clean(fields[0])] = fields[1]
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

func readRepo(path, devURL string) RepoState {
	s := RepoState{Name: filepath.Base(path), Path: path, DevURL: devURL}
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
	// PAUSED：判斷「已回覆」不能只看子字串有沒有出現——agent 自己寫問題時
	// 常會在建議選項裡提到「回覆『## 人類回覆』節」這種說明文字，也可能先
	// 附上空白的 `## 人類回覆（請在此下作答）` 範本讓人類直接編輯檔案；
	// 兩種情況子字串都存在，但都還沒真的被回覆。改成：只認「行首就是
	// `## 人類回覆`」的標題行（排除引號裡提到它的說明句），取最後一個
	// 這樣的標題（真正的回覆一定是後來附加、在檔案最尾端），再看它底下
	// 是否有非空白內容——有內容才算真的回覆過。
	if b, err := os.ReadFile(filepath.Join(ai, "PAUSED")); err == nil {
		s.Paused = true
		lines := strings.Split(string(b), "\n")
		headingIdx := -1
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "## 人類回覆") {
				headingIdx = i
			}
		}
		if headingIdx >= 0 {
			s.PausedAnswered = strings.TrimSpace(strings.Join(lines[headingIdx+1:], "\n")) != ""
			s.PausedQuestion = strings.TrimSpace(strings.Join(lines[:headingIdx], "\n"))
		} else {
			s.PausedAnswered = false
			s.PausedQuestion = strings.TrimSpace(string(b))
		}
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
	// 儀表板：有舊快照可讀，或設了 -dashboard-script 可以現算，就給連結
	_, err = os.Stat(filepath.Join(ai, "reports", "dashboard.html"))
	s.DashboardReady = err == nil || dashboardScriptPath != ""
	return s
}

// ---------- claude 帳號用量（帳號層級，跟哪個 repo 無關；60 秒快取） ----------

type UsageState struct {
	SessionPct int    `json:"session_pct"` // -1 = 未知
	WeekPct    int    `json:"week_pct"`
	FetchedAt  string `json:"fetched_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

var (
	usageMu    sync.Mutex
	usageCache UsageState
	usageAt    time.Time
)

var usagePctRe = regexp.MustCompile(`[0-9]+%`)

func parseUsagePct(text, label string) int {
	prefix := "Current session"
	if label == "week" {
		prefix = "Current week"
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, prefix) {
			m := usagePctRe.FindString(line)
			if m == "" {
				return -1
			}
			n, err := strconv.Atoi(strings.TrimSuffix(m, "%"))
			if err != nil {
				return -1
			}
			return n
		}
	}
	return -1
}

// fetchUsage 執行一次 `claude -p "/usage"`（cwd 隨便挑一個已註冊的 repo，
// 用量是帳號層級、跟 cwd 無關，只是需要在某個目錄下跑）。
func fetchUsage(repos []string) UsageState {
	if len(repos) == 0 {
		return UsageState{SessionPct: -1, WeekPct: -1, Error: "沒有已註冊的 repo"}
	}
	cmd := exec.Command("claude", "-p", "/usage", "--output-format", "json")
	cmd.Dir = repos[0]
	out, err := cmd.Output()
	if err != nil {
		return UsageState{SessionPct: -1, WeekPct: -1, Error: "claude -p /usage 執行失敗"}
	}
	var m map[string]any
	if json.Unmarshal(out, &m) != nil {
		return UsageState{SessionPct: -1, WeekPct: -1, Error: "無法解析 claude 輸出"}
	}
	result, _ := m["result"].(string)
	if result == "" {
		return UsageState{SessionPct: -1, WeekPct: -1, Error: "claude 輸出沒有 result 欄位"}
	}
	return UsageState{
		SessionPct: parseUsagePct(result, "session"),
		WeekPct:    parseUsagePct(result, "week"),
		FetchedAt:  time.Now().Format("15:04:05"),
	}
}

func getUsage(repos []string) UsageState {
	usageMu.Lock()
	defer usageMu.Unlock()
	if time.Since(usageAt) < 60*time.Second && usageAt != (time.Time{}) {
		return usageCache
	}
	usageCache = fetchUsage(repos)
	usageAt = time.Now()
	return usageCache
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
