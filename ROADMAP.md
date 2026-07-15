# ROADMAP.md — 願景對照與下一階段

> 這份文件回答兩個問題：原始 13 元件的產品願景，現在落地到哪裡了
> （§1-2）；接下來要做什麼、刻意不做什麼（§3-5）。協定與 schema
> 權威定義仍在 [`AI-RUNTIME.md`](AI-RUNTIME.md)，操作方式在
> [`MANUAL.md`](MANUAL.md)——本檔不重複，只做盤點與規劃。

## 1. 願景對照表

一次全倉稽核核對了原始願景的 13 個元件，結論：**全部已存在**。

| 元件 | 實作位置 | 狀態 |
|---|---|---|
| `.ai/` workspace 結構 | `templates/ai/`（CONTRACT.md、schedule.yml、agents/ 五角色、tasks/ 三檔、state/ 四檔、rubrics/ 四份、receipts/、reports/） | 完整 |
| CONTRACT.md 長期規則＋人類核可邊界 | `templates/ai/CONTRACT.md` §7；permission deny 雙重強制 | 完整 |
| 任務佇列 | schema 於 AI-RUNTIME.md；選取演算法在 `templates/skills/work/SKILL.md` 步驟 2（priority→FIFO，`depends_on`，`attempts`） | 完整 |
| checkpoint 持久狀態 | `templates/ai/state/checkpoint.json`；無狀態 session＋整檔重寫，恢復與正常啟動同一條路 | 完整 |
| Supervisor 自我恢復 | `supervisor/supervisor.sh`（分類器 9 類、rate-limit 睡到 reset、網路指數退避、成本斷路器、STOP 開關、quota 軟/硬門檻、watchdog、`--self-test`） | 完整 |
| Rubrics 自評 | `templates/ai/rubrics/` 四份，0-100 加權，≥80 過、60-79 改一輪、防吹牛條款 | 完整 |
| Receipts 稽核 | schema 於 AI-RUNTIME.md；`receipts/YYYY-MM-DD/NNN.md` | 完整 |
| Reports | `.claude/skills/ai-report/`（日報/週報/PR 描述/changelog/履歷素材） | 完整 |
| Multi-agent | 五 persona 在單一 `/work` session 內分工＋獨立 `/review` round（`templates/skills/review/SKILL.md`，`supervisor --review` 觸發）；平行寫入者刻意不做（single-writer invariant） | 完整（範圍已收斂，見 §2） |
| GitHub 整合 | `.claude/skills/ai-ship/`（唯一碰網路的 skill，僅限人類觸發） | 完整 |
| Dashboard | `panel/`（Go 控制台：多 repo 狀態、回答 PAUSED、STOP、帳號用量）＋ `supervisor/dashboard.sh`（零額度靜態 HTML） | 完整 |
| 事件收集 | 分散式：receipts frontmatter、`ai/queue` git log、`.ai/supervisor/` run 紀錄、`state/decisions.md` | 完整但分散（見 §2） |
| schedule.yml | `templates/ai/schedule.yml` | 存在，但目前是 supervisor 調參檔，非時間排程（見 §2） |

## 2. 誠實落差

三個「完整」但名實或範圍有落差的地方，值得記下來而不是假裝沒有：

- **schedule.yml 名實不符**：檔名暗示時間排程，實際內容是
  supervisor 的調參 key（quota 門檻、cost cap、iteration 上限）。目前
  沒有任何東西讀它來決定「幾點啟動」——啟動永遠靠人類手動觸發。
  D1 會補上這塊，讓名字對得起內容。
- **「multi-agent」是 persona 分工，不是平行 agent**：五個角色
  （planner/coder/tester/reviewer/architect）在同一個 `/work` session
  裡依序切換視角，`/review` 是另一個獨立 session 但仍是*讀者*角色、
  不碰程式碼。這是刻意設計，不是縮水——single-writer invariant 是
  checkpoint/resume 可信的前提，平行寫入者會直接打破它（見 §4）。
- **事件散落於各檔，無統一事件層**：想知道「supervisor 這輪為什麼睡了
  20 分鐘」，答案在 `run.log` 文字裡，不是結構化資料；想知道「這個
  repo 這週的任務事件時間軸」，要橫跨 receipts + git log + run.log
  自己拼。D2 要補的就是這條 loop 層事件流。

## 3. 下一階段（進行中）

以下工作包正在實作，依代號列出：

**C — 健壯性**
- **C1 rate-limit 偵測強化**：fixture 先行再放寬 classifier regex；
  `sleep_until_reset` 支援 `limit reached|<epoch>` 形式的訊息。
- **C2 狀態檔機械 lint**：supervisor 加 `lint_checkpoint`/`lint_tasks`
  ——偵測歸 supervisor，修復仍歸 agent 協定自癒；`done.yaml` 救不回時
  改名保留不清空（不能用會遺失稽核軌跡的方式復原）。
- **C3 環境體檢**：`supervisor.sh --doctor`（樹完整性、`{{` 模板殘留、
  deny-drift、巢狀 session 警告）＋ `--probe`（headless 寫入權限實測，
  補上 AI-RUNTIME.md 已知限制 4 提到的驗證缺口）。
- **C4 allowlist 補洞**：`Bash(date:*)`（時間戳協定強制要求）、
  `Bash(git show:*)`（`/review` 需要讀歷史 diff）。

**V — 定位驗證**
- **V1 第二 agent 相容性驗證（Codex CLI）**：讓 Codex 接手一份既有的
  `.ai/` workspace 完成一個任務循環（讀 CONTRACT → 選任務 → 實作 →
  receipt → done.yaml → 印 AIOS_STATUS），**先後接手、不是平行**
  （single-writer 不動）。這是「agent-agnostic」從設計目標升級為
  已驗證主張的門檻；在此之前對外措辭一律是「協定除 skill 格式外
  不假設 Claude 專屬功能」。預期產出：一份差距清單（哪些東西其實
  是 Claude 耦合：skill 載入方式、權限模型、/usage 解析）＋
  AI-RUNTIME 的「最小 agent 契約」一節。

**D — 排程與觀測**
- **D1 `supervisor/schedule-install.sh`**：launchd 產生器，讀
  `schedule.yml` 新 key `schedule_start_times`，讓 schedule.yml 名實
  相符（見 §2 第一條）。
- **D2 `.ai/supervisor/events.jsonl`**：supervisor 機械發出 loop 層
  事件（rate-limit 睡眠、quota 煞車、watchdog、每迭代成本）；
  `/ai-report` 與 `dashboard.sh` 聚合讀取。三層事件模型定案：
  task 層＝receipts、code 層＝git log、loop 層＝`events.jsonl`。

## 4. 刻意不做

延續 README.md「Positioning」一節的立場——這是協定層，不是 agent
runtime，以下項目不在範圍內：

- **平行寫入者**：會打破 single-writer invariant，checkpoint/resume
  的可信度建立在「任何時刻只有一個 session 改 `.ai/`」上。
- **messaging gateways（Telegram/Discord/Slack…）**：那是通用助理
  框架的問題，不是「讓 coding agent 在這個 repo 裡負責任地工作」的
  問題。
- **model routing**：委派給 Claude Code（`claude -p "/work"`），本
  repo 不重造 agent loop 或 model 選擇邏輯。
- **tool 生態系**：同上，工具執行是 Claude Code 的職責。
- **PreToolUse/PostToolUse hook 強制層**：目前的強制手段是
  permission deny 規則（`.claude/settings.local.json`）——它是
  best-effort 而非沙箱（誠實條款 1），但多一層版本耦合的 hook
  換不到質變，只是重複建置。
- **LLM 寫事件檔**：事件必須是機械發出（supervisor shell 直接寫
  `events.jsonl`），LLM 產生結構化日誌不可靠，這正是「已知限制 2」
  要避免的錯誤重演。
- **log rotation**：`.ai/supervisor/` 是 gitignored 執行狀態，量小
  且非長期資產，不值得引入 rotation 邏輯。
- **cron/視窗模式排程**：用作業系統原生的 launchd（D1），不用 cron
  或自製排程視窗——沒理由重造作業系統已經穩定提供的東西。
- **真 YAML parser 依賴**：`tasks/*.yaml` 走「整檔重寫＋壞檔自癒」
  策略（AI-RUNTIME.md checkpoint 規則），刻意不引入外部 parser 依賴
  以保持 supervisor 是純 shell、零安裝依賴。

## 5. 最大風險

Claude Code 原生功能的演化——background tasks、scheduled cloud
routines、原生 task queue——很可能會取代 supervisor 與 panel 現在做
的事。這在 README.md 已經點名，這裡重申並收斂成一句話：這兩塊被刻意
保持得很薄（純 shell、零額度 dashboard），因為它們本來就預期會被
Claude Code 原生功能蓋過去。真正要帶走、不會被蓋過去的資產是**協定
本身與 receipts 證據紀律**——contract 核可邊界、任務 schema、
receipt-centric 稽核、quota braking 這些東西，跟底層 runtime 是誰
無關，甚至有機會搬到其他 CLI agent 上（`.ai/` 協定除了 skill 格式
外不假設任何 Claude 專屬功能）。
