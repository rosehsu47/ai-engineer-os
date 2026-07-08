# supervisor.sh — 無人監督迴圈

讓 agent 在你離開電腦後持續工作：每輪開一個全新的 `claude -p "/work"`
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
| `--wait-on-pause` | PAUSED 時每 5 分鐘輪詢而不是退出 |
| `--dry-run` | 印出將執行的設定與指令，不花額度 |
| `--self-test` | 零額度：用內嵌 fixtures 驗證錯誤分類器與睡眠計算 |

參數預設值都在目標 repo 的 `.ai/schedule.yml`（扁平 key）。

## 錯誤分類與復原（實作在 `classify()`，self-test 有全套 fixtures）

| 訊號 | 分類 | 動作 |
|---|---|---|
| `AIOS_STATUS: STOPPED` / `.ai/STOP` | 手動停止 | exit 0 |
| `AIOS_STATUS: PAUSED` / `.ai/PAUSED` | 需要人類 | 印出問題，exit 2（`--wait-on-pause` 則輪詢） |
| `AIOS_STATUS: QUEUE_EMPTY` | 佇列空 | exit 0 |
| `DONE_TASK / TASK_PARTIAL / BLOCKED / CONTRACT_HALT` | 有生產力 | 歸零失敗計數，睡 20s 續跑 |
| `usage limit` / `resets 8pm` 等 | rate limit | 解析 reset 時間睡到該時 +2 分（上限 6h；解析失敗睡 30 分）；**不計失敗** |
| `ECONNRESET / 529 / fetch failed` 等 | 網路 | 指數退避 30s→900s，最多 6 次 |
| exit 124/137/143（watchdog 砍掉） | 逾時 | 失敗 +1，重啟 |
| 其他非零 exit / `is_error` | 未知崩潰 | 失敗 +1，睡 60s 重試 |
| exit 0 但沒有 AIOS_STATUS 行 | 協定漂移 | 警告，累計 3 次退出（檢查 /work skill 是否還在目標 repo） |

## 安全閥

- **單 repo 單 supervisor**（`.ai/supervisor/lock`，pid 存活偵測）
- `max_iterations_per_run`（預設 10）、`max_consecutive_failures`（3）
- 每輪 watchdog（預設 30 分鐘，macOS 無 timeout 用背景輪詢實作）
- **成本熔斷**：累計每輪回報的 `total_cost_usd` 超過
  `max_cost_per_run_usd`（預設 5）即停（訂閱制下數字是估算值，仍有效）
- **交叉驗證**：agent 回報有進展但 checkpoint mtime 沒動 → 當失敗計
  （防 agent 忘記協定空轉）
- `.ai/STOP` 隨時手動煞車；所有輪次的原始輸出留在 `.ai/supervisor/`

## 已知限制（誠實條款）

1. **rate-limit 偵測靠 CLI 訊息字串**（`You've hit your usage limit ·
   resets 8pm (Asia/Taipei)` 這類格式），CLI 改版可能失效——失效時會落到
   「未知崩潰」分類，行為退化成有界重試，不會爆走。
2. **權限**：預設 `acceptEdits` + 目標 repo 的 Bash allowlist。agent 用到
   白名單外的指令會被無聲拒絕，任務通常以 blocked/failed 收場——去
   `.claude/settings.local.json` 補白名單。注意 `acceptEdits` 只放行
   **cwd 內**的檔案編輯（`--add-dir` 的目錄不算），所以 supervisor 一律
   cd 進目標 repo 再呼叫 claude。
3. jq 有裝解析較穩；沒有則退回 grep/sed 保守萃取。
4. 成本數字來自 claude CLI 回報的 `total_cost_usd`，訂閱制下為推估值。
