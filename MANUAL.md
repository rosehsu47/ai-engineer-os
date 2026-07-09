# AI Engineer OS 使用說明書

> 這份是**操作手冊**：怎麼裝、怎麼跑、出事怎麼辦。
> 協定與 schema 的權威定義在 [`AI-RUNTIME.md`](AI-RUNTIME.md)；
> supervisor 細節在 [`supervisor/README.md`](supervisor/README.md)。

## 1. 系統總覽

```
                        你（人類）
                          │
        ┌─────────────────┼──────────────────────┐
        │ 安裝/種任務       │ 隨時介入              │ 出貨
        ▼                 ▼                      ▼
   /ai-init         .ai/STOP（煞車）          /ai-ship
        │           .ai/PAUSED（回答問題）    （push + GitHub PR）
        ▼                                        ▲
┌───────────────── 目標 repo ─────────────────┐  │
│  .ai/  ←──────── 唯一的狀態載體 ──────────┐  │  │
│   ├ CONTRACT.md（規則）  ├ tasks/（佇列）  │  │  │
│   ├ state/（斷點/記憶）  ├ receipts/（收據）│──┼──┘
│   └ rubrics/ agents/ schedule.yml reports/ │  │
│  .claude/skills/{work,review}（隨倉出貨）   │  │
└─────────────────────────────────────────────┘  │
        ▲                 ▲                      │
        │ 每輪全新 session  │ 審查輪（可選）        │
   claude -p "/work"  claude -p "/review"        │
        ▲                 ▲                      │
        └────── supervisor.sh（迴圈+復原+熔斷）───┘
                          │
                 dashboard.sh（零額度 HTML 儀表板）
```

核心心智模型：**agent session 是無狀態、可拋棄的**；所有狀態都在 `.ai/`
檔案裡。所以 crash、rate limit、你手動關機，恢復方式都一樣——再開一輪就好。

## 2. 首次安裝（每個 repo 一次）

```
 cd work-record-tool ── claude ── /ai-init /path/to/repo
                                      │
                     ┌────────────────┴───────────────┐
                     │ 檢查：git repo？.ai/ 不存在？    │
                     └────────────────┬───────────────┘
                                      ▼
                        複製 templates/ai/ → {repo}/.ai/
                        複製 work/review skills → {repo}/.claude/skills/
                                      ▼
                        訪談 6 題（使命/測試指令/建置指令/
                        主分支/特有禁令/.ai/ 要不要 commit）
                                      ▼
                        填 CONTRACT + 合併權限 allowlist
                                      ▼
                        commit「chore(ai): initialize …」
```

裝完後**先做一件事**：打開 `{repo}/.ai/CONTRACT.md` 讀一遍——這是 agent
的憲法，訪談沒問到的細節（例如「哪些目錄絕不能碰」）現在補進 §4。

## 3. 種任務

**推薦方式（對話引導，不用手寫 YAML）**：

```bash
cd /path/to/repo && claude
/ai-task 幫我把匯出功能的逾時問題修掉
```

它會用選擇題確認 type/priority、**替你起草可驗證的 acceptance 條件**讓你挑、
太大的任務建議拆小，最後預覽 YAML 確認才寫入。

手動方式：編輯 `{repo}/.ai/tasks/backlog.yaml`（schema 見 AI-RUNTIME.md）。
寫好任務的三個判準：

- `acceptance` 每一條都**可客觀驗證**（「跑 `make version` 會輸出短 hash」，
  不是「程式碼品質良好」）
- 一個任務 = 一個 session 做得完的量（做不完的拆小，用 `depends_on` 串）
- `priority: 1` 是最高；同 priority 先進先出

## 4. 執行——一輪長什麼樣（/work）

```
        claude -p "/work"
              │
   ┌──────────▼──────────┐   STOP/PAUSED 存在？──▶ 直接結束（不寫任何檔）
   │ 0 守門               │   完全無法寫入？────▶ 印 BLOCKED（免寫出口）
   └──────────┬──────────┘
              ▼
   1 讀契約/斷點/情境 ──── checkpoint 壞掉？→ 重置自癒
              ▼
   2 續作或選任務 ──────── doing 有任務/斷點非 idle → 從 task_step 接手
              │            佇列空 → 印 QUEUE_EMPTY 結束
              ▼
   3 認領（backlog → doing，attempts+1）
              ▼
   4 契約預檢 ──────────── 碰禁止操作 → done(abandoned)+CONTRACT_HALT
              │            跨人類批准界線 → 寫 .ai/PAUSED + PAUSED
              ▼
   5 實作（ai/queue 分支，最小變更，逐步更新 task_step）
              ▼
   6 測試 ──── 失敗 → 修（最多 2 輪）──仍敗→ WIP commit + 退回/abandoned
              ▼ 通過                          + receipt(failed) + TASK_PARTIAL
   7 自評 rubric ── <80 → 改一輪重評 ── 仍 <80 → 以 partial 出貨（不第三輪）
              ▼ ≥80
   8 提交儀式（自檢 diff → type(scope): title [T-NNN]）
              ▼
   9 記錄（receipt → done.yaml → checkpoint 歸 idle → context/memory
          → chore(ai): records for T-NNN）
              ▼
  10 印 AIOS_STATUS: DONE_TASK …，結束（一輪只做一個任務）
```

手動跑一輪（建議首次先這樣觀察）：

```bash
cd /path/to/repo && claude -p "/work"
```

## 5. 無人監督（supervisor）

```bash
supervisor/supervisor.sh --repo /path/to/repo --once      # 先單輪
supervisor/supervisor.sh --repo /path/to/repo             # 正式（預設 ≤10 輪）
supervisor/supervisor.sh --repo /path/to/repo --review    # 每任務加獨立審查
```

```
     啟動 ──▶ lock？STOP？PAUSED？──▶ 跑一輪 /work（watchdog 30m）
                                          │
                                    讀 AIOS_STATUS / 錯誤徵兆分類
                                          │
   ┌──────────┬───────────┬──────────────┼───────────┬───────────────┐
   ▼          ▼           ▼              ▼           ▼               ▼
 DONE/     QUEUE_      PAUSED         rate limit   網路錯誤        崩潰/逾時
 PARTIAL   EMPTY      （印問題        （睡到 reset  （指數退避      （失敗+1，
   │        │          exit 2）       +2 分，不計   30s→900s，     60s 重試）
   │        ▼                         失敗，連續    最多 6 次）        │
   │      正常收工                     8 輪上限）                     │
   ▼                                                                 │
 [--review] 開 fresh session /review ── FAIL → 修正任務自動排入 ◀────┘
   │                                            backlog
   ▼
 睡 20s → 下一輪（直到：佇列空 / 達輪數上限 / 連續失敗 3 次 /
                  累計成本 > US$5 / 你 touch .ai/STOP）
```

**你隨時可以介入**：
- `touch {repo}/.ai/STOP` → 當輪結束後停（刪掉檔案即恢復）
- agent 留了 `.ai/PAUSED` → 在該 repo 跑 **`/ai-answer`**：它把 agent 的
  問題呈現成選擇題、把你的決定記到正確的地方（任務描述/記憶）、刪掉
  PAUSED、問你要不要立刻重啟（手動流程：讀檔→處理→刪檔，也可以）
- 看狀態：`supervisor/dashboard.sh --repo /path` → 開
  `.ai/reports/dashboard.html`

### 人性化互動的五個入口（都是問答/選擇題/按鈕，不用碰 YAML）

| 情境 | 入口 | 互動形式 |
|---|---|---|
| 首次安裝 | `/ai-init {repo}` | 訪談 6 題填契約 |
| 交辦工作 | `/ai-task {一句話描述}` | 選擇題定 type/priority、挑 acceptance 草稿 |
| agent 卡住等你 | `/ai-answer` 或 **panel 問答區** | 問題＋選項呈現，回覆走同一條協定路徑 |
| 總覽與煞車 | **`aios-panel`**（[panel/README.md](panel/README.md)） | 網頁：多 repo 狀態卡、就地回答、STOP 按鈕 |
| 出貨 | `/ai-ship {repo}` | PR 草稿先確認再推送（panel 只提示可出貨數） |

**panel 快速上手**：`cd panel && go run . -repos /path/a,/path/b` →
開 http://127.0.0.1:7777。回覆的協定：任何介面把回答附加成 PAUSED 的
`## 人類回覆` 節，下一輪 /work 自行路由並繼續——panel、/ai-answer、
甚至手機上直接編輯檔案，走的都是同一條路。

## 6. 出貨與收成

```
 累積了幾個 DONE_TASK
        │
        ▼
 /ai-ship /path ──▶ 確認 PR 草稿 ──▶ push ai/queue + gh pr create
        │                                    │
        ▼                                    ▼
 /ai-report /path weekly              GitHub 上 review + merge
        │                             （merge 永遠是你按的）
        ▼
 .ai/reports/weekly-*.md
  ├ PR 描述草稿 / Changelog
  └ 履歷素材 ──▶ /new-project-intro 會自動吸收（CONVENTIONS §8）
                 → my-summary.md / 面試 talking points
```

## 7. 故障快查

| 症狀 | 先看 | 處置 |
|---|---|---|
| supervisor 說 PAUSED | `{repo}/.ai/PAUSED` 內容 | 回答/處理後**刪掉該檔**重啟 |
| 任務一直 failed | 該任務最後一張 receipt 的「證據」節 + `.ai/state/memory.md` | 修 acceptance 或拆小任務；達 max_attempts 會進 done(abandoned) |
| 回報 BLOCKED（權限） | `.ai/supervisor/out.json` 的 denials | 補 `{repo}/.claude/settings.local.json` 白名單；首跑檢查清單見 AI-RUNTIME 已知限制 |
| 協定漂移警告 | `{repo}/.claude/skills/work/` 還在嗎 | 重新從 templates 複製 skill |
| 想全部重來 | — | `.ai/` 是普通檔案：git revert 或整包刪掉重新 /ai-init |
| 懷疑 supervisor 本身 | `supervisor.sh --self-test`（零額度） | 13 個 fixtures 應全過 |

## 8. 成本觀念

每輪 = 一個全新 session（重讀契約與狀態 ~3-5k tokens + 實作 + 測試）。
安全網依序是：`max_iterations_per_run`（10）→ `max_cost_per_run_usd`（5）→
rate-limit 自動睡眠 → 你的 STOP。想省：任務寫小寫清楚（減少修正輪）、
`review_after_task` 只在重要 repo 開。
