# AI-RUNTIME.md — AI Engineer OS 協定權威文件

> 這份文件定義 `.ai/` runtime workspace 的所有 schema 與協定。
> 任何 skill、supervisor、或人類對格式有疑問時，**以這份為準**。
> 對應的操作入口：`/ai-init`（安裝）、`/work`（執行一輪）、`/ai-report`（產報告）、
> `supervisor/supervisor.sh`（無人監督迴圈）。

## 系統圖

```
   supervisor.sh ──每輪開全新 session──▶ claude -p "/work"
        │                                    │
        │ 讀 AIOS_STATUS / 分類錯誤 / 復原      │ 讀寫 .ai/（唯一的狀態載體）
        ▼                                    ▼
   .ai/supervisor/*（log/lock/cost）    .ai/{tasks,state,receipts,...}
```

核心原則：**所有狀態都在檔案裡**。agent session 是無狀態的、可拋棄的；
crash / rate-limit / 手動中斷之後的恢復，跟正常啟動是同一條路徑。

## `.ai/` 目錄結構

```
.ai/
├── CONTRACT.md          agent 的長期規則（/ai-init 訪談後填成）
├── schedule.yml         supervisor 參數（扁平 key）
├── tasks/
│   ├── backlog.yaml     待辦佇列
│   ├── doing.yaml       進行中（不變量：至多 1 筆）
│   └── done.yaml        已完成（append-only）
├── state/
│   ├── checkpoint.json  斷點（每個子步驟後重寫）
│   ├── context.md       滾動工作情境（≤100 行）
│   ├── memory.md        長期教訓（可重用的知識才進來）
│   └── decisions.md     架構決策紀錄（ADR-lite）
├── rubrics/             自評量表（依 task type 選用）
├── agents/              /work 各步驟採用的 persona 定義
├── receipts/YYYY-MM-DD/NNN.md   每個任務的審計收據
├── reports/             /ai-report 產出
├── supervisor/          supervisor 執行狀態（gitignored）
├── STOP                 （存在即要求全面停止；人類手動建立/刪除）
└── PAUSED               （存在即等待人類；內容 = agent 的具體問題）
```

## AIOS_STATUS 協定

`/work` 每次執行的**最後一行輸出**必須是：

```
AIOS_STATUS: <STATUS> task=<id|none> score=<0-100|na> receipt=<相對路徑|none>
```

實例：`AIOS_STATUS: DONE_TASK task=T-001 score=100 receipt=receipts/2026-07-08/001.md`
（receipt 欄 = 相對 `.ai/` 的完整路徑；receipt frontmatter 內的自我編號
則是短形 `"2026-07-08/001"`。）

| STATUS | 意義 | supervisor 的動作 |
|---|---|---|
| `DONE_TASK` | 完成一個任務 | 繼續下一輪 |
| `TASK_PARTIAL` | 任務部分完成/測試修不動已退回 | 繼續下一輪 |
| `QUEUE_EMPTY` | 佇列沒有可執行任務 | 正常結束（exit 0） |
| `BLOCKED` | 無法進行：權限/環境阻擋（含「完全無法寫入」的免寫出口——此時什麼檔案都不碰，只印狀態行） | 繼續下一輪 |
| `PAUSED` | 需要人類（.ai/PAUSED 已寫入問題） | 印出問題後停止（exit 2） |
| `STOPPED` | .ai/STOP 存在 | 立即結束（exit 0） |
| `CONTRACT_HALT` | 任務要求被契約禁止的操作 | 繼續下一輪（換任務） |

## AIOS_REVIEW 協定（多 agent 審查輪，可選）

`schedule.yml` 的 `review_after_task: true`（或 supervisor `--review`）時，
每個 `DONE_TASK` 之後 supervisor 會開一個**全新 session** 跑 `/review`：
獨立重評上一個任務（fresh context，非實作者自評）。輸出最後一行：

```
AIOS_REVIEW: <PASS|FAIL> task=<id> followup=<新任務id|none>
```

FAIL 時 reviewer **不自己修**——把修正任務（priority 1）排進 backlog，
下一輪 /work 自然接手；審查判定同時附加在受審 receipt 尾端。
這維持了單一寫手不變量：任何時刻只有一個 session 在改程式碼。

## checkpoint.json schema

```json
{
  "version": 1,
  "updated_at": "2026-07-08T10:00:00+08:00",
  "iteration": 0,
  "phase": "idle",
  "current_task_id": null,
  "task_step": null,
  "last_completed_task_id": null,
  "last_receipt": null,
  "last_commit": null,
  "eval": { "attempts": 0, "last_score": null },
  "blocked_reason": null,
  "session_notes": null
}
```

- `phase` ∈ `idle | selecting | executing | testing | evaluating | committing`
- `task_step`：自由文字，精確描述「目前做到哪個子步驟」——中途被殺時，
  下一個 session 從這裡續作，不重頭
- `session_notes`：≤3 行，下一個 session 必須知道的事
- **規則：整檔重寫，不做局部修補**。JSON 壞掉時：從本 schema 重置、
  在 memory.md 記一筆、繼續工作（自癒優先於追究）

## tasks/*.yaml schema

```yaml
# backlog.yaml
version: 1
tasks:
  - id: T-001              # T-NNN，遞增不重用
    title: ""
    priority: 1            # 1 = 最高
    type: feature          # feature|fix|chore|test|docs|architecture|performance
    description: ""
    acceptance:            # 可客觀驗證的完成條件，至少一條
      - ""
    depends_on: []         # 這些 id 必須已在 done.yaml
    status: pending        # pending | blocked
    blocked_reason: null
    attempts: 0
    max_attempts: 3
    created_at: "2026-07-08T10:00:00+08:00"
```

- `doing.yaml`：同結構 + `started_at`。**不變量：至多 1 筆**（單 agent）。
  發現多於 1 筆＝前一輪異常中斷：把多餘的退回 backlog 再繼續。
- `done.yaml`：同結構 + `finished_at`、`result`（done|partial|abandoned）、
  `receipt`（收據相對路徑）。**append-only，永不改寫既有條目**。
  `abandoned` = 任務達 max_attempts 仍失敗、或被契約終結——移進來
  而不是留在 backlog 當殭屍，/ai-report 的「待人類處理」節會列出它們。
- YAML 一律**整檔重寫**（同 checkpoint 規則）。

## Receipt 格式 — `receipts/YYYY-MM-DD/NNN.md`

NNN = 當日流水號，補零三位（列目錄取最大值 +1）。

```markdown
---
receipt: "2026-07-08/001"
task_id: T-001
title: ""
status: done            # done | partial | blocked | failed | paused
started_at: ""
finished_at: ""
commit: null            # 程式碼 commit 的 sha；無則 null
files_changed: []
tests: { command: "", result: "pass", summary: "" }   # result: pass|fail|skipped
rubric: { name: null, score: null, threshold: 80, attempts: 0 }
---

## 做了什麼

## 為什麼（決策理由）

## 證據
（測試輸出摘錄 + `git diff --stat`；沒有證據的宣稱不要寫）

## 未盡事項與風險
```

frontmatter 供 `/ai-report` 機器讀取；prose 供人類與履歷管線使用。

## Git 紀律（寫進每份 CONTRACT，此處為協定層規定）

- 工作分支：CONTRACT `環境資訊` 指定（預設 `ai/queue`），不存在則從主分支建立
- **只 `git add` 自己為了這個任務修改的檔案；禁止 `git add -A` / `git add .`**
  （目標 repo 可能有人類未 commit 的工作）
- 程式碼 commit：`type(scope): title [T-NNN]`；`.ai/` 紀錄另開
  `chore(ai): records for T-NNN`
- 禁止 force push、禁止 commit 到主分支

## 已知限制（誠實條款）

1. **檔案禁令的強制力**：CONTRACT 的禁止事項對 agent 是指令不是沙箱。
   硬防線只有 `.claude/settings.local.json` 的 permission deny 規則
   （由 /ai-init 安裝，擋 `.claude/**` 與 `.ai/CONTRACT.md` 的編輯與
   白名單外的 Bash）。deny 規則的具體行為隨 Claude Code 版本演進，
   高風險 repo 請勿使用 `--yolo`。
2. **LLM 寫壞 YAML/JSON 是遲早的事**：協定用「整檔重寫 + 壞檔自癒重置 +
   supervisor 檢查 checkpoint mtime 前進」三層緩解，但沒有 schema validator。
3. **rate-limit 偵測依賴 CLI 訊息格式**（如
   `You've hit your usage limit · resets 8pm (Asia/Taipei)`），版本間可能改變；
   supervisor 解析失敗時退回固定睡眠，所有未知錯誤都收斂到「有界重試」。
4. **headless 寫入權限未經真實終端驗證**（2026-07-08 建置時的實測侷限）：
   從巢狀 Claude session 呼叫 `claude -p` 時，子 session 的檔案寫入被父層
   權限系統攔下，因此「/work 在 headless 下成功寫檔」只驗證到協定層
   （被擋時 agent 正確回報 BLOCKED、還原 stash、零損害），沒驗證到放行路徑。
   **首次使用檢查清單**：從一般終端機（不在 Claude session 內）跑
   `supervisor.sh --repo X --once`；若 receipts/輸出顯示權限阻擋，
   依序嘗試 (a) 確認目標 repo `.claude/settings.local.json` 有
   `Edit(**)`/`Write(**)` allow 條目，(b) `--yolo`（信任的 repo 才用）。
   /work 演算法本身已逐步實跑驗證（產物鏈：雙 commit、receipt、
   checkpoint、done.yaml 全部正確）。
