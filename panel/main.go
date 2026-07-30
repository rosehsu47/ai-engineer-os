// aios-panel — AI Engineer OS 的本機控制台（零外部依賴，只綁 127.0.0.1）。
//
// 設計原則：panel 只是「協定檔的讀者與寫者」，判斷力留在 agent——
//
//	讀：supervisor lock（幾個 agent 在跑）、doing/backlog/done、checkpoint、
//	    PAUSED、last_run、receipts frontmatter、ai/queue 領先數
//	寫：只寫兩種協定檔——PAUSED 的「## 人類回覆」節、.ai/STOP 的建立/刪除
//	出貨（push）是對外動作，panel 只顯示可出貨數量與 /ai-ship 指令。
//	唯一例外：claude 帳號用量（/api/usage）——帳號層級、不是協定檔，
//	查一次要 spawn `claude -p "/usage"`（非零成本，~0.5s），所以刻意
//	跟 5 秒的 state 輪詢分開：60 秒快取一次，且只查一次（不分 repo）。
//	另一個例外：「可出貨」判斷背景跑 `git fetch origin`（唯讀、只更新
//	remote-tracking ref，不碰本地分支/工作區）——GitHub 上 PR 合併後，
//	不 fetch 的話本地 main 會一直落後，「可出貨」數字就卡在合併前的舊值。
//	每個 repo 最多 60 秒 fetch 一次，離線/失敗就跳過不擋畫面。
//
// 用法：
//
//	go run ./panel -repos /path/a,/path/b        （或編譯後 aios-panel）
//	沒給 -repos 時讀 ~/.aios-repos（一行一個 repo 路徑，# 開頭為註解）
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
	Name             string   `json:"name"`
	Path             string   `json:"path"`
	Missing          bool     `json:"missing"` // .ai/ 不存在
	SupervisorAlive  bool     `json:"supervisor_alive"`
	SupervisorPID    int      `json:"supervisor_pid,omitempty"`
	Stopped          bool     `json:"stopped"` // .ai/STOP 存在
	Phase            string   `json:"phase"`
	Iteration        int      `json:"iteration"`
	CurrentTask      string   `json:"current_task"` // doing.yaml 的 id+title
	Backlog          []string `json:"backlog"`      // 前 5 筆 "T-NNN title"
	BacklogCount     int      `json:"backlog_count"`
	DoneCount        int      `json:"done_count"`
	Paused           bool     `json:"paused"`
	PausedQuestion   string   `json:"paused_question,omitempty"`
	PausedAnswered   bool     `json:"paused_answered"`
	Shippable        int      `json:"shippable"`   // ai/queue 領先主分支的 commit 數
	DirtyCount       int      `json:"dirty_count"` // working tree 未 commit 的檔案數（未記帳警訊）
	LastRunStatus    string   `json:"last_run_status,omitempty"`
	LastRunCost      string   `json:"last_run_cost,omitempty"`
	LastRunAt        string   `json:"last_run_at,omitempty"`
	Receipts         []string `json:"receipts"`              // 最近 3 張 "日期/NNN [status] [human]? title"
	DashboardReady   bool     `json:"dashboard_ready"`       // 卡片要不要顯示「儀表板」連結
	DevURL           string   `json:"dev_url,omitempty"`     // ~/.aios-repos 該行第二欄（本機 dev server 網址，可選）
	DevCommand       string   `json:"dev_command,omitempty"` // ~/.aios-repos 該行第三欄起（啟動 dev server 的指令，可選）
	DevServerRunning bool     `json:"dev_server_running"`
	DevServerPID     int      `json:"dev_server_pid,omitempty"`
}

// DevConfig：~/.aios-repos 每行選填的第二、三欄——dev server 網址與啟動指令。
// 純人工維護，不是 `.ai/` 協定檔，agent 不讀不寫。
type DevConfig struct {
	URL     string
	Command string
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

	lanInfo := ""
	if ip := firstLANIP(); ip != "" {
		port := (*addr)[strings.LastIndex(*addr, ":")+1:]
		lanInfo = fmt.Sprintf("內網位址（僅供參考——panel 只綁 127.0.0.1，這個位址目前連不進來）：http://%s:%s", ip, port)
	}
	renderedPage := strings.Replace(pageHTML, "{{LAN_INFO}}", lanInfo, 1)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderedPage)
	})
	http.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		repos := currentRepos()
		devCfg := loadDevConfig(*reposFlag)
		states := make([]RepoState, 0, len(repos))
		for _, p := range repos {
			states = append(states, readRepo(p, devCfg[p]))
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

	http.HandleFunc("/api/devserver", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		repo, action := r.FormValue("repo"), r.FormValue("action")
		if !allowed(currentRepos(), repo) {
			http.Error(w, "unknown repo", 400)
			return
		}
		cfg := loadDevConfig(*reposFlag)[filepath.Clean(repo)]
		switch action {
		case "start":
			if cfg.Command == "" {
				http.Error(w, "此 repo 在 ~/.aios-repos 沒有設定啟動指令（第三欄）", 400)
				return
			}
			if err := startDevServer(repo, cfg); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		case "stop":
			if err := stopDevServer(repo, cfg.URL); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		default:
			http.Error(w, "action 必須是 start|stop", 400)
			return
		}
		w.Write([]byte("ok"))
	})
	http.HandleFunc("/api/devlog", func(w http.ResponseWriter, r *http.Request) {
		repo := r.URL.Query().Get("repo")
		if !allowed(currentRepos(), repo) {
			http.Error(w, "unknown repo", 400)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeFile(w, r, devLogFile(repo))
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

// loadDevConfig 讀 ~/.aios-repos 每行第二欄起（空白分隔，皆可選）：本機
// dev server 網址（第二欄）與啟動指令（第三欄起，joined）。純人工維護，
// agent 不讀不寫、不是協定檔。格式：`{path} {url} {command...}`，例如
// `/repo/a http://localhost:5173 npm run dev`。只有網址沒有指令就不會
// 顯示啟動按鈕。-repos flag 給的清單不支援這兩欄（CLI 用法維持簡單，
// 回傳空 map）。
func loadDevConfig(flagVal string) map[string]DevConfig {
	out := map[string]DevConfig{}
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
		if len(fields) < 2 {
			continue
		}
		cfg := DevConfig{URL: fields[1]}
		if len(fields) > 2 {
			cfg.Command = strings.Join(fields[2:], " ")
		}
		out[filepath.Clean(fields[0])] = cfg
	}
	return out
}

// firstLANIP：抓第一個看起來像區網位址的非 loopback IPv4（僅供標題列
// 顯示參考用，panel 實際綁定位址不受這個值影響）。
func firstLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		ip4 := ipnet.IP.To4()
		if ip4 == nil || ip4.IsLinkLocalUnicast() {
			continue
		}
		return ip4.String()
	}
	return ""
}

func allowed(repos []string, p string) bool {
	for _, r := range repos {
		if r == filepath.Clean(p) {
			return true
		}
	}
	return false
}

// ---------- 背景 git fetch（每 repo 最多 60 秒一次，讓 shippable 判斷跟得上 GitHub 上的合併） ----------

const fetchInterval = 60 * time.Second

var (
	fetchMu   sync.Mutex
	lastFetch = map[string]time.Time{}
)

// fetchOriginIfStale：距上次 fetch 超過 fetchInterval 才真的跑，避免每
// 5 秒的 state 輪詢都打一次網路。同步執行（有 5 秒逾時保護）——只在
// 每個 repo 每 60 秒的第一次請求會稍微變慢，其餘輪詢直接用快取結果。
func fetchOriginIfStale(path string) {
	fetchMu.Lock()
	stale := time.Since(lastFetch[path]) > fetchInterval
	if stale {
		lastFetch[path] = time.Now()
	}
	fetchMu.Unlock()
	if !stale {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exec.CommandContext(ctx, "git", "-C", path, "fetch", "origin", "--quiet").Run()
}

// ---------- dev server 管理（panel 自己的狀態，刻意不寫進目標 repo 的
// .ai/——那裡只給協定檔用，啟動指令是純本機執行狀態，跟審計/協定無關）----------

// panelStateDir：pid/log 檔案存放處，跟目標 repo 完全分開。
func panelStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".aios-panel-state")
	os.MkdirAll(dir, 0o755)
	return dir
}

var devIDRe = regexp.MustCompile(`[^A-Za-z0-9]+`)

func devServerID(path string) string {
	return strings.Trim(devIDRe.ReplaceAllString(path, "_"), "_")
}

func devPidFile(path string) string { return filepath.Join(panelStateDir(), devServerID(path)+".pid") }
func devLogFile(path string) string { return filepath.Join(panelStateDir(), devServerID(path)+".log") }

// ownPidStatus：讀 pidfile 並確認該 pid 真的還活著（同 supervisor lock
// 的判斷手法）。pidfile 存在但程序已死＝上次沒有正常關閉，視為未執行。
// 只反映「panel 自己啟動、還追蹤得到 pid」的那個行程。
func ownPidStatus(path string) (bool, int) {
	b, err := os.ReadFile(devPidFile(path))
	if err != nil {
		return false, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return false, 0
	}
	if syscall.Kill(pid, 0) != nil {
		os.Remove(devPidFile(path)) // 陳舊 pidfile，順手清掉
		return false, 0
	}
	return true, pid
}

// portInUse：試著自己聽那個 port，聽得到就代表沒人占用（立刻關掉釋放），
// 聽不到就是已經有人在用。用來偵測「不是 panel 啟動、但已經在跑」的
// 服務（例如你自己在終端機手動開的）——不能只看 pidfile，否則對沒帶
// 明確 --port 的指令（例如 `next dev` 沒寫死 port），panel 會以為沒在
// 跑而重新啟動，Next.js 發現 port 被占用會自己悄悄換一個 port，變成
// 使用者搞不清楚哪個才是真的在跑的那個。
func portInUse(devURL string) bool {
	u, err := url.Parse(devURL)
	if err != nil || u.Port() == "" {
		return false // 沒有明確 port 就無法判斷，不擋
	}
	ln, err := net.Listen("tcp", ":"+u.Port())
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

// devServerStatus：給畫面顯示與「啟動前檢查」用——panel 自己追蹤得到的
// pid 優先；追不到但 port 已經有人占用，也算「執行中」（pid 回 0，代表
// 沒有 panel 可以控制的 pid，只能告知使用者，不能幫忙關）。
func devServerStatus(path, devURL string) (bool, int) {
	if running, pid := ownPidStatus(path); running {
		return true, pid
	}
	if devURL != "" && portInUse(devURL) {
		return true, 0
	}
	return false, 0
}

// startDevServer：`sh -c command` 在自己的 process group 起（Setpgid），
// 這樣 stop 時可以用 -pid 把整棵子行程樹（例如 `npm run dev` 底下真正
// 幹活的 `next-server`）一起收掉，不留孤兒行程。輸出導去 log 檔，
// 背景 goroutine Wait() 回收，行程結束時順手清 pidfile（讓「當掉的
// dev server」下一輪輪詢就會正確顯示成未執行，不用等使用者手動按停止）。
// **啟動前一定先檢查 port 有沒有人在用**（不論是不是 panel 自己啟動
// 的）——已經有人占用就直接視為「已經在跑」，不重複啟動，避免對沒寫
// 死 port 的指令（如純 `next dev`）造成它自己悄悄換到別的 port。
func startDevServer(path string, cfg DevConfig) error {
	if running, _ := devServerStatus(path, cfg.URL); running {
		return nil
	}
	logf, err := os.OpenFile(devLogFile(path), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command("sh", "-c", cfg.Command)
	cmd.Dir = path
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logf.Close()
		return err
	}
	if err := os.WriteFile(devPidFile(path), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		logf.Close()
		return err
	}
	go func() {
		cmd.Wait()
		logf.Close()
		os.Remove(devPidFile(path))
	}()
	return nil
}

// stopDevServer：SIGTERM 整個 process group，給 2 秒優雅關閉，逾時才
// SIGKILL。負 pid＝殺整個 group（Setpgid 保證這個 group 只有這棵樹）。
// 只能關 panel 自己追蹤到 pid 的那個；如果偵測到 port 有人占用但不是
// 我們啟動的，明確告知使用者，不假裝關掉了。
func stopDevServer(path, devURL string) error {
	running, pid := ownPidStatus(path)
	if !running {
		os.Remove(devPidFile(path))
		if devURL != "" && portInUse(devURL) {
			return fmt.Errorf("這個 port 上有服務在跑，但不是 panel 啟動的（沒有追蹤到 pid），需要你自己手動關掉")
		}
		return nil
	}
	syscall.Kill(-pid, syscall.SIGTERM)
	for range 20 {
		if syscall.Kill(pid, 0) != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		syscall.Kill(-pid, syscall.SIGKILL)
	}
	os.Remove(devPidFile(path))
	return nil
}

// ---------- 讀取協定檔（容錯優先：壞檔回空值，不 panic） ----------

func readRepo(path string, devCfg DevConfig) RepoState {
	s := RepoState{Name: filepath.Base(path), Path: path, DevURL: devCfg.URL, DevCommand: devCfg.Command}
	if devCfg.Command != "" {
		s.DevServerRunning, s.DevServerPID = devServerStatus(path, devCfg.URL)
	}
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
	costs := taskCosts(filepath.Join(ai, "supervisor"))
	s.CurrentTask = withCost(firstTask(filepath.Join(ai, "tasks", "doing.yaml")), costs)
	backlog, backlogCount := taskList(filepath.Join(ai, "tasks", "backlog.yaml"), 5)
	for i, t := range backlog {
		backlog[i] = withCost(t, costs)
	}
	s.Backlog, s.BacklogCount = backlog, backlogCount
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
	// shippable：ai/queue 領先主分支多少 commit。優先比 origin/<branch>
	// （PR 在 GitHub 合併後這裡最準，不必等人手動 git pull）；沒有
	// remote-tracking ref（離線、沒 remote）就退回本地分支
	fetchOriginIfStale(path)
	for _, base := range []string{"origin/main", "origin/master", "main", "master"} {
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
	s.Receipts = recentReceipts(filepath.Join(ai, "receipts"), 3, costs)
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

// taskCosts 讀 .ai/supervisor/events.jsonl，依 task id 加總 cost_usd
// （iteration 事件是原始 /ai-work 那輪，review 事件是同一個任務的審查
// 輪——兩者都歸帳到同一個 task id，見 AI-RUNTIME.md 事件模型）。這是
// 執行遙測而非稽核檔：檔案不存在、被 truncate、或某筆任務全程走人類
// 互動 session（/ai-task 種進去但用 /ai-wrap 收帳，從沒被 supervisor
// 跑過）都查無資料，一律回傳「這筆沒有」而非報錯。
func taskCosts(supDir string) map[string]float64 {
	out := map[string]float64{}
	f, err := os.Open(filepath.Join(supDir, "events.jsonl"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var ev struct {
			Task    string  `json:"task"`
			CostUsd float64 `json:"cost_usd"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Task == "" || ev.Task == "none" || ev.CostUsd == 0 {
			continue
		}
		out[ev.Task] += ev.CostUsd
	}
	return out
}

func fmtCost(v float64) string { return "$" + strconv.FormatFloat(v, 'f', 2, 64) }

// withCost 把 "T-NNN 標題" 的任務顯示行加上該任務目前的累計成本；查無
// 資料就原樣回傳（前端 splitId 只切第一個空白，加的尾巴會落進 title）。
func withCost(line string, costs map[string]float64) string {
	if line == "" {
		return line
	}
	id, _, _ := strings.Cut(line, " ")
	if v, ok := costs[id]; ok {
		return line + " · " + fmtCost(v)
	}
	return line
}

func recentReceipts(dir string, n int, costs map[string]float64) []string {
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
		taskID := get("task_id")
		title := get("title")
		if title == "" {
			title = taskID // 舊格式收據沒有 title 欄位（如 kotoba），退回顯示 task_id 避免整行空白
		}
		if v, ok := costs[taskID]; ok {
			title += " · " + fmtCost(v)
		}
		out = append(out, fmt.Sprintf("%s [%s] %s%s", rel, get("status"), human, title))
	}
	return out
}
