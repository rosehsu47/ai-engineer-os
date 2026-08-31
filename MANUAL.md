# AI Engineer OS 使用說明書

> 這份是**操作手冊**：怎麼裝、怎麼跑、出事怎麼辦。
> 協定與 schema 的權威定義在 [`AI-RUNTIME.md`](AI-RUNTIME.md)；
> supervisor 細節在 [`supervisor/README.md`](supervisor/README.md)。

## 1. 系統總覽

```mermaid
flowchart TD
    H(("你（人類）"))
    H -->|"安裝/種任務"| INIT["/ai-init<br/>/ai-task"]
    H -->|"隨時介入"| PANEL["aios-panel（網頁控制台）<br/>· 狀態總覽/待辦/收據<br/>· 回答 PAUSED（textarea）<br/>· STOP 煞車按鈕<br/>· 啟動 supervisor（表單）"]
    H -->|"出貨"| SHIP["/ai-ship<br/>(push + GitHub PR)"]

    subgraph REPO["目標 repo"]
        AI[".ai/ ← 唯一的狀態載體<br/>CONTRACT.md（規則） · tasks/（佇列）<br/>state/（斷點/記憶） · receipts/（收據）<br/>rubrics/ agents/ schedule.yml reports/"]
        SKILLS[".claude/skills/{ai-work,ai-review}（隨倉出貨）"]
    end

    INIT --> REPO
    PANEL -.->|"回答/煞車 = /ai-answer、touch STOP 的同一條協定檔路徑"| AI
    AI --> SHIP

    SUP["supervisor.sh<br/>（迴圈+復原+熔斷）"]
    SUP -->|"每輪全新 session"| WORK["claude -p /ai-work"] --> REPO
    SUP -->|"審查輪（可選）"| REVIEW["claude -p /ai-review"] --> REPO
    SUP --> DASH["dashboard.sh<br/>（零額度 HTML 儀表板）"]
```

核心心智模型：**agent session 是無狀態、可拋棄的**；所有狀態都在 `.ai/`
檔案裡。所以 crash、rate limit、你手動關機，恢復方式都一樣——再開一輪就好。

## 2. 首次安裝（每個 repo 一次）

```mermaid
flowchart TD
    A["cd ai-engineer-os ── claude ── /ai-init /path/to/repo"]
    B{"檢查：git repo？<br/>.ai/ 不存在？"}
    C["複製 templates/ai/ → {repo}/.ai/<br/>複製 ai-work/ai-review skills → {repo}/.claude/skills/"]
    D["訪談 6 題<br/>（使命/測試指令/建置指令/<br/>主分支/特有禁令/.ai/ 要不要 commit）"]
    E["填 CONTRACT + 合併權限 allowlist"]
    F["commit「chore(ai): initialize …」"]
    A --> B --> C --> D --> E --> F
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

## 4. 執行——一輪長什麼樣（/ai-work）

```mermaid
flowchart TD
    START(["claude -p /ai-work"]) --> S0{"0 守門"}
    S0 -->|"STOP/PAUSED 存在"| END0(["直接結束（不寫任何檔）"])
    S0 -->|"完全無法寫入"| END1(["印 BLOCKED（免寫出口）"])
    S0 -->|"通過"| S1["1 讀契約/斷點/情境<br/>（checkpoint 壞掉 → 重置自癒）"]
    S1 --> S2{"2 續作或選任務"}
    S2 -->|"doing 有任務/斷點非 idle"| S3
    S2 -->|"佇列空"| END2(["印 QUEUE_EMPTY 結束"])
    S2 -->|"否則"| S3["3 認領<br/>（backlog → doing，attempts+1）"]
    S3 --> S4{"4 契約預檢"}
    S4 -->|"碰禁止操作"| END3(["done(abandoned) + CONTRACT_HALT"])
    S4 -->|"跨人類批准界線"| END4(["寫 .ai/PAUSED，印 PAUSED"])
    S4 -->|"通過"| S5["5 實作<br/>（ai/queue 分支，最小變更，逐步更新 task_step）"]
    S5 --> S6{"6 測試"}
    S6 -->|"失敗，≤2 輪修正仍敗"| END5(["WIP commit + 退回/abandoned<br/>+ receipt(failed) + TASK_PARTIAL"])
    S6 -->|"通過"| S7{"7 自評 rubric"}
    S7 -->|"低於 80，改一輪重評仍不足"| S8
    S7 -->|"≥80"| S8["8 提交儀式<br/>（自檢 diff → type(scope): title T-NNN）"]
    S8 --> S9["9 記錄<br/>receipt → done.yaml → checkpoint 歸 idle<br/>→ context/memory → chore(ai): records for T-NNN"]
    S9 --> END6(["印 AIOS_STATUS: DONE_TASK …，結束<br/>（一輪只做一個任務）"])
```

手動跑一輪（建議首次先這樣觀察）：

```bash
cd /path/to/repo && claude -p "/ai-work"
```

## 5. 無人監督（supervisor）

```bash
supervisor/supervisor.sh --doctor --repo /path/to/repo    # 首跑前體檢（零額度）
supervisor/supervisor.sh --repo /path/to/repo --once      # 先單輪
supervisor/supervisor.sh --repo /path/to/repo             # 正式（預設 ≤10 輪）
supervisor/supervisor.sh --repo /path/to/repo --review    # 每任務加獨立審查
supervisor/schedule-install.sh --repo /path/to/repo       # 固定時刻自動啟動
#   （時刻設在 .ai/schedule.yml 的 schedule_start_times，launchd 排程）
```

```mermaid
flowchart TD
    A["啟動"] --> B{"lock？STOP？<br/>PAUSED？quota？"}
    B --> C["跑一輪 /ai-work<br/>（watchdog 30m）"]
    C --> D["讀 AIOS_STATUS ／ 錯誤徵兆分類"]
    D --> E1["DONE/PARTIAL"]
    D --> E2["QUEUE_EMPTY"]
    D --> E3["PAUSED"]
    D --> E4["rate limit"]
    D --> E5["網路錯誤"]
    D --> E6["崩潰/逾時"]

    E2 --> F1(["正常收工"])
    E3 --> F2(["印問題，exit 2"])
    E4 --> F3(["睡到 reset +2 分，不計失敗<br/>（連續 8 輪上限）"])
    E5 --> F4(["指數退避 30s→900s，最多 6 次"])
    E6 --> F5(["失敗+1，60s 重試"])

    E1 --> G{"--review？"}
    G -->|"是"| H["開 fresh session /ai-review"]
    H -->|"FAIL"| I["修正任務自動排入 backlog"] --> J
    H -->|"PASS"| J
    G -->|"否"| J["睡 20s → 下一輪"]
    F5 --> J

    J --> K["直到：佇列空 / 達輪數上限 / 連續失敗 3 次 /<br/>累計成本 > US$20 / quota 硬門檻 / 你 touch .ai/STOP"]
```

**quota 檢查**（每輪跑 `/ai-work` 前，圖中的「lock？STOP？PAUSED？quota？」那步）：
5h 用量 ≥60% 就不開新任務，每 20 分查一次 `/usage` 等降回來自動續跑——
任務不會中途斷頭，5h 就算衝到 100% 也是等，不會停；硬門檻 80% 只看 7d，
達標才寫 STOP 保個人額度（7d 要等數天不划算）。皆可在 `schedule.yml` 調；
想有意衝額度就加 `--ignore-quota`，這次不查也不寫 STOP，連上次 quota 寫的
舊 STOP 都會一併清掉再開跑。

**你隨時可以介入**：
- **停滯復原**：看 `.ai/supervisor/run.log` 找退出原因，然後重跑
  `supervisor.sh --repo {repo}` 就是恢復（checkpoint 續作，殘留 lock 自動清）；
  完整 SOP 見 [supervisor/README.md](supervisor/README.md)
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
| 總覽與煞車 | **`aios-panel`**（[panel/README.md](panel/README.md)） | 網頁：多 repo 狀態卡、就地回答、STOP 按鈕、啟動 supervisor |
| 出貨 | `/ai-ship {repo}` | PR 草稿先確認再推送（panel 只提示可出貨數） |

**panel 快速上手**：`cd panel && go run . -repos /path/a,/path/b` →
開 http://127.0.0.1:7777。回覆的協定：任何介面把回答附加成 PAUSED 的
`## 人類回覆` 節，下一輪 /ai-work 自行路由並繼續——panel、/ai-answer、
甚至手機上直接編輯檔案，走的都是同一條路。

## 6. 出貨與收成

```mermaid
flowchart TD
    A["累積了幾個 DONE_TASK"] --> B["/ai-ship /path"]
    B --> C["確認 PR 草稿"] --> D["push ai/queue + gh pr create"]
    D --> E["GitHub 上 review + merge<br/>（merge 永遠是你按的）"]
    A --> F["/ai-report /path weekly"]
    F --> G[".ai/reports/weekly-*.md<br/>· PR 描述草稿 / Changelog<br/>· 履歷素材"]
    G -->|"自動吸收（CONVENTIONS §8）"| H["/new-project-intro<br/>→ my-summary.md / 面試 talking points"]
```

## 7. 故障快查

| 症狀 | 先看 | 處置 |
|---|---|---|
| 不確定環境對不對 | `supervisor.sh --doctor --repo X`（零額度，一般終端機跑） | 照各 ❌ 附的修法處理；allow/deny drift 報 missing → 手動補進該 repo 的 settings.local.json |
| supervisor 說 PAUSED | `{repo}/.ai/PAUSED` 內容 | 回答/處理後**刪掉該檔**重啟 |
| 任務一直 failed | 該任務最後一張 receipt 的「證據」節 + `.ai/state/memory.md` | 修 acceptance 或拆小任務；達 max_attempts 會進 done(abandoned) |
| 回報 BLOCKED（權限） | `.ai/supervisor/out.json` 的 denials | 補 `{repo}/.claude/settings.local.json` 白名單；`--doctor --probe` 可直接定位（AI-RUNTIME 已知限制 4） |
| 協定漂移警告 | `{repo}/.claude/skills/ai-work/` 還在嗎 | 重新從 templates 複製 skill |
| 想全部重來 | — | `.ai/` 是普通檔案：git revert 或整包刪掉重新 /ai-init |
| 懷疑 supervisor 本身 | `supervisor.sh --self-test`（零額度） | 全部 fixtures 應過 |

## 8. 成本觀念

每輪 = 一個全新 session（重讀契約與狀態 ~3-5k tokens + 實作 + 測試）。
**實測量級（2026-07-17，kotoba 7 任務）**：一個中型任務 $1.7-4.6、
中位數約 $2-3；整個 backlog（7 任務全一次通過）約 $20。數字是訂閱制下
CLI 的推估值，當相對量看。
安全網依序是：`max_iterations_per_run`（10）→ `max_cost_per_run_usd`
（預設 20 ≈ 5-8 個任務）→ rate-limit 自動睡眠 → 你的 STOP。
想省：任務寫小寫清楚（減少修正輪）、`review_after_task`（預設開，
每任務多 ~$1-2）在不重要的 repo 關掉——但大型變更（>10 檔）的強制
review 不受此設定影響。
