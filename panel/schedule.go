// schedule.go — 讀寫 .ai/schedule.yml 的表單化版本（跟 /ai-config skill
// 是同一份資料的兩種操作方式：/ai-config 是終端機引導式問答附解釋，這裡
// 是 panel 網頁上直接填表存檔，兩者都只動 schedule.yml 這一個檔案）。
//
// schedule.yml 是扁平 key（不能巢狀），且是 supervisor.sh 只「讀」的
// 設定檔（agent 也不能碰它——settings.local.json 的 deny 名單上），沒有
// 並發寫入風險，寫入不用檢查 supervisor lock／session lock，跟
// /api/answer、/api/stop 那種要小心 .ai/ 併發寫入的協定檔不一樣。
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// scheduleField：表單一個欄位的定義——key 跟 supervisor.sh sched_get()
// 讀的名字一致，Default 是 sched_get() 沒讀到值時的實際 fallback（跟
// supervisor/README.md 的參考表同一份數字，改動請兩邊一起改）。
type scheduleField struct {
	Key      string
	Label    string
	Category string
	Kind     string // "int" | "bool" | "text" | "model"
	Default  string
	Help     string
}

// scheduleFields：順序即表單顯示順序，按 /ai-config skill 同一套五類分組。
var scheduleFields = []scheduleField{
	{"max_iterations_per_run", "單次執行最多跑幾輪", "執行安全閥", "int", "10", "這次執行最多跑幾輪 /ai-work 就正常收工（不是失敗，是安全上限）"},
	{"max_consecutive_failures", "連續非生產性結果幾次就停", "執行安全閥", "int", "3", "連續幾次「非生產性」結果（crash、未知崩潰，不含 rate limit）就整個停下"},
	{"iteration_timeout_minutes", "單輪逾時分鐘數", "執行安全閥", "int", "30", "單輪 /ai-work 的 watchdog 逾時，超過強制砍掉，算失敗 +1"},
	{"max_cost_per_run_usd", "單次執行花費上限（美元）", "執行安全閥", "int", "20", "累計花費超過即熔斷停止（約 5–8 個任務量；訂閱制下是推估值）"},
	{"quota_wait_threshold_pct", "5h 額度軟門檻（%）", "quota 額度門檻", "int", "60", "只看 5h 額度，達標就不開新任務，等降回才續跑（沒有上限；0＝停用）"},
	{"quota_stop_threshold_pct", "7d 額度硬門檻（%）", "quota 額度門檻", "int", "80", "只看 7d 額度，達標即寫 .ai/STOP 保週額度（101＝停用）"},
	{"quota_wait_recheck_minutes", "軟門檻等待期重查間隔（分）", "quota 額度門檻", "int", "20", "軟門檻等待期間每隔幾分鐘重查一次 /usage"},
	{"review_after_task", "每任務後開獨立審查", "審查與暫停行為", "bool", "true", "每個 DONE_TASK 後要不要開全新 session 獨立審查（多花 ~$1-2）"},
	{"wait_on_pause", "PAUSED 時輪詢而不退出", "審查與暫停行為", "bool", "false", "撞到未回覆的 .ai/PAUSED 時要不要輪詢而不是退出"},
	{"pause_poll_interval_seconds", "PAUSED 輪詢間隔（秒）", "審查與暫停行為", "int", "30", "wait_on_pause 開啟時的輪詢間隔秒數"},
	{"claude_model", "model", "model／flags／排程時刻", "model", "sonnet", "/ai-work 用哪個 model 跑"},
	{"extra_claude_flags", "附加給 claude CLI 的 flags", "model／flags／排程時刻", "text", "", "空白分隔，值裡不可含空白；留白＝不附加"},
	{"schedule_start_times", "每天固定啟動時刻", "model／flags／排程時刻", "text", "", "給 supervisor/schedule-install.sh 讀去產生 launchd job；例：09:00,21:30，留白＝不排程"},
	{"rate_limit_fallback_sleep_minutes", "rate limit 解析失敗固定睡幾分鐘", "網路重試參數（很少需要調）", "int", "30", "撞到 rate limit 但解析不出 reset 時間時，固定睡多久"},
	{"network_backoff_base_seconds", "網路錯誤退避起始秒數", "網路重試參數（很少需要調）", "int", "30", "網路錯誤（ECONNRESET/529 等）指數退避的起始秒數"},
	{"network_backoff_max_seconds", "網路錯誤退避上限秒數", "網路重試參數（很少需要調）", "int", "900", "網路錯誤指數退避的上限秒數，最多 6 次"},
}

var allowedScheduleModels = map[string]bool{"opus": true, "sonnet": true, "haiku": true}

// scheduleLineRe(key)：扁平 key 的那一行，值一律在同一行、不跨行——跟
// /ai-config skill 的假設一致。
func scheduleLineRe(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:.*$`)
}

// readScheduleValues：解析 .ai/schedule.yml 的扁平 key: value（跳過註解、
// 空行），只認 scheduleFields 裡列出的 key；缺的 key 回傳空字串，由呼叫端
// 補 Default 顯示。壞檔/不存在回傳空 map，不 panic。
func readScheduleValues(path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(filepath.Join(path, ".ai", "schedule.yml"))
	if err != nil {
		return out
	}
	known := map[string]bool{}
	for _, f := range scheduleFields {
		known[f.Key] = true
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rawKey, rawVal, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(rawKey)
		if !known[key] {
			continue
		}
		val := strings.TrimSpace(rawVal)
		val = strings.Trim(val, `"`)
		out[key] = val
	}
	return out
}

// writeScheduleValues：只精準取代每個有改動的 key 那一行（保留其餘行與
// 註解不動，不整檔重寫、不重新產生註解文字——跟 /ai-config skill 步驟 4
// 同一個紀律）；key 原本不存在就在檔尾補一行。updates 只包含要改的 key。
func writeScheduleValues(path string, updates map[string]string) error {
	p := filepath.Join(path, ".ai", "schedule.yml")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	content := string(b)
	for _, f := range scheduleFields {
		v, ok := updates[f.Key]
		if !ok {
			continue
		}
		line := f.Key + ": " + v
		if f.Kind == "model" || f.Kind == "text" {
			line = f.Key + `: "` + v + `"`
		}
		re := scheduleLineRe(f.Key)
		if re.MatchString(content) {
			// ReplaceAllLiteralString，不是 ReplaceAllString——後者的替換
			// 字串裡 $ 會被當成 submatch 參照，extra_claude_flags／
			// schedule_start_times 這種自由輸入欄位使用者打的值可能真的
			// 含 $，用 literal 版本才不會被吃掉或誤解析。
			content = re.ReplaceAllLiteralString(content, line)
		} else {
			content = strings.TrimRight(content, "\n") + "\n" + line + "\n"
		}
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

// validateScheduleUpdate：跟 /api/supervisor 同一套紀律——數字欄位驗證
// 是合法整數，model 欄位限制白名單，文字欄位只擋換行（單行 key，值跨行
// 會弄壞下一行的解析）。回傳清乾淨、只含實際有變動欄位的 map。
func validateScheduleUpdate(form map[string]string) (map[string]string, string) {
	out := map[string]string{}
	for _, f := range scheduleFields {
		v, ok := form[f.Key]
		if !ok {
			continue
		}
		switch f.Kind {
		case "int":
			if strings.TrimSpace(v) == "" {
				return nil, f.Key + " 不能留白"
			}
			if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
				return nil, f.Key + " 必須是整數"
			}
			out[f.Key] = strings.TrimSpace(v)
		case "bool":
			if v != "true" && v != "false" {
				return nil, f.Key + " 必須是 true 或 false"
			}
			out[f.Key] = v
		case "model":
			if !allowedScheduleModels[v] {
				return nil, "claude_model 不合法"
			}
			out[f.Key] = v
		case "text":
			if strings.ContainsAny(v, "\n\r") {
				return nil, f.Key + " 不能包含換行"
			}
			out[f.Key] = v
		}
	}
	return out, ""
}
