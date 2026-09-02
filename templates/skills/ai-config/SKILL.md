---
name: ai-config
description: 用問答方式調整這個 repo 的 .ai/schedule.yml（supervisor 參數）——不用手改扁平 key YAML。人類互動用。用法：/ai-config
---

# /ai-config — 引導式調整 schedule.yml（人類互動）

把「我想要 supervisor 用什麼參數跑」變成一次問答，不逼使用者手改
`.ai/schedule.yml` 的扁平 key。**只動這一個檔案**——不碰 CONTRACT.md、
tasks/*.yaml、state/*，那些是 `/ai-task`／`/ai-wrap`／`/ai-work` 的事。

**不用檢查 supervisor lock／session lock**：`schedule.yml` 是
`supervisor.sh` 只「讀」不「寫」的設定檔（agent 也不能碰它——在
`settings.local.json` 的 deny 名單上），沒有並發寫入風險，跟
`/ai-task`／`/ai-wrap` 需要防跟 supervisor 同時整檔重寫 `tasks/*.yaml`
的情況不一樣。

## 流程

### 步驟 0：讀現況

讀 `.ai/schedule.yml`（不存在就提示「先跑過 `/ai-init` 才有這個檔案」，
結束）。逐一列出目前每個 key 的值；缺的 key 就標注「未設定（沿用
supervisor.sh 的預設值 X）」——下面每個 key 旁邊列的預設值就是
`sched_get()` 的實際 fallback，跟 `supervisor/README.md` 的參考表一致。

用一段簡短摘要呈現目前設定給使用者看一眼（不用等他問）。

### 步驟 1：選要調整的類別

用 AskUserQuestion（multiSelect）問使用者這次想調哪些類別，只問被選中
類別底下的 key，沒選的維持原樣：

- **執行安全閥**：`max_iterations_per_run`、`max_consecutive_failures`、
  `iteration_timeout_minutes`、`max_cost_per_run_usd`
- **quota 額度門檻**：`quota_wait_threshold_pct`、`quota_stop_threshold_pct`、
  `quota_wait_recheck_minutes`
- **審查與暫停行為**：`review_after_task`、`wait_on_pause`、
  `pause_poll_interval_seconds`
- **model／flags／排程時刻**：`claude_model`、`extra_claude_flags`、
  `schedule_start_times`
- **網路重試參數**（很少需要調，選項說明寫「進階，通常不用動」）：
  `rate_limit_fallback_sleep_minutes`、`network_backoff_base_seconds`、
  `network_backoff_max_seconds`

### 步驟 2：逐類別問

每個類別用 AskUserQuestion（一次最多 4 題，類別內超過 4 個 key 就分兩次
問），每題選項第一個永遠是「維持目前值 X」，其餘放 2–3 個常見選項，
使用者也可以選 Other 自己輸入。每題附一句解釋（意譯自
`supervisor/README.md` 的 schedule.yml 參考表，不用逐字照抄）：

- `max_iterations_per_run`（預設 10）：這次執行最多跑幾輪 `/ai-work`
  就正常收工。選項：5（輕量測試）／10／20（長時間放著跑）
- `max_consecutive_failures`（預設 3）：連續幾次非生產性結果就整個
  停下。選項：3／5（較寬容）／1（嚴格）
- `iteration_timeout_minutes`（預設 30）：單輪 watchdog 逾時分鐘數。
  選項：15（任務通常很快）／30／60（任務常常複雜）
- `max_cost_per_run_usd`（預設 20）：累計花費超過這個金額即熔斷停止
  （約 5–8 個任務量）。選項：10（保守）／20／50（長時間放著跑）
- `quota_wait_threshold_pct`（預設 60）：5h 額度用量達標就不開新任務，
  等降回才續跑（沒有上限；0 = 停用）。選項：40（額度緊，早點等）／60／
  80（額度多，晚點才等）
- `quota_stop_threshold_pct`（預設 80）：7d 額度用量達標即寫 STOP 保
  週額度（101 = 停用）。選項：80／90（較晚停）／101（停用）
- `quota_wait_recheck_minutes`（預設 20）：軟門檻等待期間每隔幾分鐘
  重查一次 `/usage`。選項：10／20／30
- `review_after_task`（預設 true）：每個 `DONE_TASK` 後要不要開全新
  session 獨立審查（多花 ~$1-2）。選項：開（推薦，無人看管時多一雙
  眼睛）／關（省成本）
- `wait_on_pause`（預設 false）：撞到未回覆的 `.ai/PAUSED` 時要不要
  輪詢而不是退出。選項：關（預設，手動盯著跑）／開（無人看管長跑的
  repo 建議開）
- `pause_poll_interval_seconds`（預設 30）：`wait_on_pause` 開啟時的
  輪詢間隔秒數。選項：30／10（更快接上回答）／60
- `claude_model`（預設 sonnet）：`/ai-work` 用哪個 model 跑。選項：
  sonnet／opus／haiku
- `extra_claude_flags`（預設空）：附加給 `claude` CLI 的原始 flags，
  空白分隔、值裡不可含空白。自由輸入，留白 = 不附加
- `schedule_start_times`（預設空）：固定時刻自動啟動（給
  `supervisor/schedule-install.sh` 讀去產生 launchd job，`supervisor.sh`
  本身不讀這個 key）。自由輸入，格式例："09:00,21:30"，留白 = 不排程
- `rate_limit_fallback_sleep_minutes`（預設 30）：rate limit 但解析不出
  reset 時間時固定睡多久
- `network_backoff_base_seconds` / `network_backoff_max_seconds`
  （預設 30 / 900）：網路錯誤指數退避的起始／上限秒數

### 步驟 3：預覽並確認

把「這次會改的 key：舊值 → 新值」列出來給使用者看（沒被選類別、或選了
但答「維持目前值」的 key 不列，本來就不會動），用 AskUserQuestion 問
「確定要寫入嗎？」。使用者反悔或要求重選，回步驟 1。

### 步驟 4：寫入

**只用 Edit 工具精準取代每個有改動的 key 那一行**（`key: 舊值` →
`key: 新值`），保留檔案裡其餘的行與註解完全不動——不整檔重寫、不重新
產生註解文字。缺的 key 需要新增時，用 Write 讀出全檔內容、在合理位置
（同類 key 附近）插入新的一行後整檔寫回，一樣不動其他既有行。

### 步驟 5：收尾

- 檢查 `.ai/` 是否被這個 repo 的 git 追蹤（`git check-ignore .ai/schedule.yml`
  或 `git ls-files --error-unmatch`）：有追蹤 → `git add .ai/schedule.yml`
  + commit（`chore(schedule): 調整 <改了的 key 列表>`）；沒追蹤（該 repo
  `/ai-init` 訪談時選了不 commit `.ai/`）→ 告知使用者「已存檔，這個 repo
  的 `.ai/` 沒有進 git，不會有 commit」
- 提醒使用者可以用 `supervisor/supervisor.sh --repo . --dry-run` 確認
  新值有生效，或 `--doctor` 順便做一次環境體檢
