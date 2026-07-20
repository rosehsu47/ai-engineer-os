# supervisor.sh — 無人監督迴圈

讓 agent 在你離開電腦後持續工作：每輪開一個全新的 `claude -p "/ai-work"`
session，讀 `AIOS_STATUS` 分類結果，出錯時按分類表復原。
協定與 schema 見 [`../AI-RUNTIME.md`](../AI-RUNTIME.md)。

## 快速上手

```bash
# 前提：目標 repo 已跑過 /ai-init，backlog.yaml 有任務
supervisor/supervisor.sh --repo /path/to/repo --once      # 先跑一輪觀察
supervisor/supervisor.sh --repo /path/to/repo             # 正式跑（預設最多 10 輪）
touch /path/to/repo/.ai/STOP                              # 隨時煞車
```

## Flags

| flag | 說明 |
|---|---|
| `--repo <path>` | 必填，目標 repo |
| `--once` | 只跑一輪 |
| `--max-iterations N` / `--max-failures N` / `--model M` | 覆蓋 schedule.yml |
| `--claude-flags "..."` | 附加給 claude CLI 的 flags |
| `--yolo` | 用 `--dangerously-skip-permissions`（信任的 repo 才用；永不自動啟用） |
| `--review` | 每個 DONE_TASK 後開全新 session 獨立審查（= schedule 的 `review_after_task`） |
| `--wait-on-pause` | PAUSED 時每 5 分鐘輪詢而不是退出 |
| `--dry-run` | 印出將執行的設定與指令，不花額度 |
| `--self-test` | 零額度：用內嵌 fixtures 驗證錯誤分類器與睡眠計算 |
| `--doctor` | 零額度環境體檢：`.ai/` 樹、CONTRACT `{{` 殘留、settings allow/deny 對模板的 drift、skills 齊全、狀態檔結構 lint、旗標狀態。**在一般終端機跑**（巢狀 Claude session 會讓權限結果失真，doctor 會自己警告） |
| `--probe`（配 `--doctor`） | 花少量額度：spawn 一次 `claude -p` 實測 headless 寫入權限（寫 `.ai/supervisor/probe.txt`，3 分鐘 watchdog），是 AI-RUNTIME 已知限制 4 檢查清單的可執行版 |

參數預設值都在目標 repo 的 `.ai/schedule.yml`（扁平 key）。

## 錯誤分類與復原（實作在 `classify()`，self-test 有全套 fixtures）

| 訊號 | 分類 | 動作 |
|---|---|---|
| `AIOS_STATUS: STOPPED` / `.ai/STOP` | 手動停止 | exit 0 |
| `AIOS_STATUS: PAUSED` / `.ai/PAUSED` | 需要人類 | 印出問題，exit 2（`--wait-on-pause` 則輪詢） |
| `AIOS_STATUS: QUEUE_EMPTY` | 佇列空 | exit 0 |
| `DONE_TASK / TASK_PARTIAL / BLOCKED / CONTRACT_HALT` | 有生產力 | 歸零失敗計數，睡 20s 續跑 |
| `usage/session limit` / `resets 8pm` / `resets 6:50am` 等 | rate limit | 解析 reset 時間（含帶分鐘格式）睡到該時 +2 分（上限 6h；解析失敗睡 30 分）；**不計失敗** |
| `ECONNRESET / 529 / fetch failed / temporarily unavailable`（含權限分類器暫時不可用）等 | 網路 | 指數退避 30s→900s，最多 6 次 |
| exit 124/137/143（watchdog 砍掉） | 逾時 | 失敗 +1，重啟 |
| 其他非零 exit / `is_error` | 未知崩潰 | 失敗 +1，睡 60s 重試 |
| exit 0 但沒有 AIOS_STATUS 行 | 協定漂移 | 警告，累計 3 次退出（檢查 /ai-work skill 是否還在目標 repo） |

## 安全閥

- **單 repo 單 supervisor**（`.ai/supervisor/lock`，pid 存活偵測）
- `max_iterations_per_run`（預設 10）、`max_consecutive_failures`（3）
- 每輪 watchdog（預設 30 分鐘，macOS 無 timeout 用背景輪詢實作）
- **成本熔斷**：累計每輪回報的 `total_cost_usd` 超過
  `max_cost_per_run_usd`（預設 20 ≈ 5-8 個任務，實測一個任務 $2-5）
  即停（訂閱制下數字是估算值，仍有效）
- **交叉驗證**：agent 回報有進展但 checkpoint mtime 沒動 → 當失敗計
  （防 agent 忘記協定空轉）
- `.ai/STOP` 隨時手動煞車；所有輪次的原始輸出留在 `.ai/supervisor/`
- **quota 雙門檻**：每輪開跑前用 `claude -p "/usage"`（零成本、~0.5s）查 5h／7d
  用量（查不到就放行，不誤殺）：
  - **軟門檻 `quota_wait_threshold_pct`**（預設 60）：5h 用量達標就**不開新任務**，
    每 `quota_wait_recheck_minutes`（預設 20）分鐘再查，降回門檻下自動繼續——
    任務只在額度足以整輪跑完時開工，不會中途斷頭、下輪重讀浪費 token。
    只看 5h 額度，且**沒有上限**：就算 5h 衝到硬門檻甚至 100%，一樣是等待
    （會 reset、值得等），不會因為 5h 用量高就直接停下。等待逾 24h 仍未降
    （通常是個人使用持續佔用）則寫 STOP 收工。設 0 停用。
  - **硬門檻 `quota_stop_threshold_pct`**（預設 80）：只看 7d，達標即寫
    `.ai/STOP` 停下，保留個人額度（7d 要等數天，等待不划算；設 101 停用）

## schedule-install.sh — 固定時刻自動啟動（launchd）

```bash
# 1. 在目標 repo 的 .ai/schedule.yml 設定啟動時刻
#    schedule_start_times: "09:00,21:30"
supervisor/schedule-install.sh --repo /path/to/repo             # 安裝/更新
supervisor/schedule-install.sh --repo /path/to/repo --status    # 看狀態
supervisor/schedule-install.sh --repo /path/to/repo --uninstall # 移除
supervisor/schedule-install.sh --repo /path/to/repo --dry-run   # 只印 plist
```

用 macOS 原生 launchd（睡醒補跑、重開機存活），不自製排程迴圈。plist 可
隨時從 schedule.yml 重新產生，`--doctor` 會顯示 job 是否載入。三件事不變：

- **STOP 永遠贏過排程**——排程照觸發，supervisor 見 `.ai/STOP` 即退
- lock 防重疊：排程觸發時若已有 supervisor 在跑，新 run 直接退出
- agent 不能自己排程：`.ai/schedule.yml` 在 deny 名單上，只有人類能改時刻

## dashboard.sh — 靜態儀表板（零額度）

```bash
supervisor/dashboard.sh --repo /path/to/repo
open /path/to/repo/.ai/reports/dashboard.html
```

從 `.ai/`（checkpoint、任務佇列、receipts frontmatter、supervisor 狀態、
`ai/queue` 的 git log）渲染單檔 HTML：任務統計、收據表（含自評分數與
獨立審查判定）、git 事件。純 bash/awk，不呼叫任何 LLM。

## 停滯了怎麼辦（recovery SOP）

supervisor 不在跑、agent 看起來沒動時，三個檔案就能判斷發生什麼事：

```bash
tail -20 {repo}/.ai/supervisor/run.log     # supervisor 最後在做什麼、為何退出
cat {repo}/.ai/supervisor/last_run.json    # 最後一輪的分類結果
cat {repo}/.ai/state/checkpoint.json       # agent 做到哪個子步驟
```

然後**直接重跑 `supervisor.sh --repo {repo}` 就是恢復**——狀態全在檔案裡，
crash 後的恢復與正常啟動是同一條路：checkpoint 會從中斷的子步驟續作，
殘留 lock 以 pid 存活判定自動清除。特殊情況只有兩種：
- `.ai/PAUSED` 存在 → 先回答問題（panel 或 `/ai-answer`），再重跑
- `.ai/STOP` 存在 → 確認原因（quota 煞車會把原因寫在檔內）後刪掉，再重跑

若 run.log 顯示是「未知崩潰 ×3 → 退出」而輸出裡其實是額度訊息，代表
CLI 又換了 limit 文案、分類器沒認出來——把原文加進 `--self-test` 的
fixtures、放寬 `classify()` 的 regex（先加測試再改，見下方限制 1）。

## 已知限制（誠實條款）

1. **rate-limit 偵測靠 CLI 訊息字串**（已認得：`usage/session/weekly
   limit`、`resets [at] 8pm` / `6:50am`、headless 的
   `…limit reached|<unix epoch>`（直接睡到 epoch）、`rate_limit_error`、
   裸 `429`），CLI 改版可能失效——失效時會落到「未知崩潰」分類，
   行為退化成有界重試後退出，不會爆走；修法見上方 recovery SOP
   （鐵律：先加 fixture 再改 regex）。
2. **權限**：預設 `acceptEdits` + 目標 repo 的 Bash allowlist。agent 用到
   白名單外的指令會被無聲拒絕，任務通常以 blocked/failed 收場——去
   `.claude/settings.local.json` 補白名單。注意 `acceptEdits` 只放行
   **cwd 內**的檔案編輯（`--add-dir` 的目錄不算），所以 supervisor 一律
   cd 進目標 repo 再呼叫 claude。
3. jq 有裝解析較穩；沒有則退回 grep/sed 保守萃取。
4. 成本數字來自 claude CLI 回報的 `total_cost_usd`，訂閱制下為推估值。
