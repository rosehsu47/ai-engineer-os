# ROADMAP.md — 願景對照與下一階段

> 這份文件回答兩個問題：原始 13 元件的產品願景，現在落地到哪裡了
> （§1-2）；接下來要做什麼、刻意不做什麼（§3-5）。協定與 schema
> 權威定義仍在 [`AI-RUNTIME.md`](AI-RUNTIME.md)，操作方式在
> [`MANUAL.md`](MANUAL.md)——本檔不重複，只做盤點與規劃。

## 1. 願景對照表

一次全倉稽核核對了原始願景的 13 個元件，結論：**全部已存在**。

| 元件 | 實作位置 | 狀態 |
|---|---|---|
| `.ai/` workspace 結構 | `templates/ai/`（CONTRACT.md、schedule.yml、agents/ 五角色、tasks/ 三檔、state/ 四檔、rubrics/ 五份、receipts/、reports/） | 完整 |
| CONTRACT.md 長期規則＋人類核可邊界 | `templates/ai/CONTRACT.md` §7；permission deny 雙重強制 | 完整 |
| 任務佇列 | schema 於 AI-RUNTIME.md；選取演算法在 `templates/skills/ai-work/SKILL.md` 步驟 2（priority→FIFO，`depends_on`，`attempts`） | 完整 |
| checkpoint 持久狀態 | `templates/ai/state/checkpoint.json`；無狀態 session＋整檔重寫，恢復與正常啟動同一條路 | 完整 |
| Supervisor 自我恢復 | `supervisor/supervisor.sh`（分類器 9 類、rate-limit 睡到 reset（含 epoch 精確形）、網路指數退避、成本斷路器、STOP 開關、quota 軟/硬門檻、watchdog、狀態檔 lint（`lint_checkpoint`/`lint_tasks`）、`--self-test`、`--doctor`/`--probe` 環境體檢） | 完整 |
| Rubrics 自評 | `templates/ai/rubrics/` 五份（含 content-completeness，docs 類任務窮舉覆蓋度核對），0-100 加權，≥80 過、60-79 改一輪、防吹牛條款 | 完整 |
| Receipts 稽核 | schema 於 AI-RUNTIME.md；`receipts/YYYY-MM-DD/NNN.md` | 完整 |
| Reports | `.claude/skills/ai-report/`（日報/週報/PR 描述/changelog/履歷素材） | 完整 |
| Multi-agent | 五 persona 在單一 `/ai-work` session 內分工＋獨立 `/ai-review` round（`templates/skills/ai-review/SKILL.md`，`supervisor --review` 觸發）；平行寫入者刻意不做（single-writer invariant） | 完整（範圍已收斂，見 §2） |
| GitHub 整合 | `.claude/skills/ai-ship/`（唯一碰網路的 skill，僅限人類觸發） | 完整 |
| Dashboard | `panel/`（Go 控制台：多 repo 狀態、回答 PAUSED、STOP、啟動 supervisor、帳號用量）＋ `supervisor/dashboard.sh`（零額度靜態 HTML） | 完整 |
| 事件收集 | 三層各自為政（`AI-RUNTIME.md` 事件模型節，刻意不做統一匯流排）：task＝receipts、code＝git log、loop＝`.ai/supervisor/events.jsonl`（D2 已完成——`supervisor.sh` 機械發出，`/ai-report`／`dashboard.sh` 消費） | 完整 |
| schedule.yml | `templates/ai/schedule.yml` ＋ `supervisor/schedule-install.sh`（D1 已完成——讀 `schedule_start_times` 產生 launchd 排程） | 完整 |

## 2. 誠實落差

一個「完整」但名實有落差的地方，值得記下來而不是假裝沒有：

- **「multi-agent」是 persona 分工，不是平行 agent**：五個角色
  （planner/coder/tester/reviewer/architect）在同一個 `/ai-work` session
  裡依序切換視角，`/ai-review` 是另一個獨立 session 但仍是*讀者*角色、
  不碰程式碼。這是刻意設計，不是縮水——single-writer invariant 是
  checkpoint/resume 可信的前提，平行寫入者會直接打破它（見 §4）。

（原本記在這裡的另外兩條落差——schedule.yml 名實不符、事件散落無統一
層——已分別由 D1／D2 補上，見 §1 對應列與 §3 異動記錄。）

## 3. 下一階段（進行中）

以下工作包正在實作，依代號列出——**只列真正還沒做完的**（上一輪稽核
把已經上線的項目留在清單裡沒清掉，本輪核對 git log 後移除，見文末異動
記錄）。**目前優先序：V1 與 P1 排最前**——V1 決定「agent-agnostic」這個
定位敘事能不能站得住（在此之前對外只能講「協定不假設 Claude 專屬
功能」，不能講「已支援多 agent」）；P1 決定「安全跑一晚只花 $X」這個
賣點在訂閱制（最大宗使用情境，不是 API-key 計費）下是不是名副其實。
C5 是持續性健壯度工作，不像 V1/P1 那樣急迫。

**P — 定價與成本可見度**
- **P1 訂閱制下的真實成本可見度**：`supervisor.sh` 的成本熔斷全部
  依賴 claude CLI 自報的 `total_cost_usd`——這在 API-key 計費下是真帳，
  但在訂閱制（Claude Pro/Max，也是最大宗的使用情境）下只是推估值，
  已記在 `supervisor/README.md` 已知限制 4（「成本數字來自 claude CLI
  回報的 total_cost_usd，訂閱制下為推估值」），但目前沒有工作項去改善
  它，只是誠實承認。修法路徑：`events.jsonl` 的 `iteration` 事件現在
  已經帶 `usage.cache_creation_input_tokens`/`cache_read_input_tokens`/
  `input_tokens`/`output_tokens`（2026-07-28 加進 `supervisor.sh`）——
  這代表 supervisor 可以用 [`shared/models.md`](shared/models.md)
  的公開定價表（cache write ≈1.25x/2x、cache read ≈0.1x base input，
  2026-09-03 已建檔，來源見該檔「資料來源與新鮮度」節）自己重新算一次
  成本，不必只信任 CLI 自報的 `total_cost_usd`，兩者對不上時至少能標記
  「這輪成本是估算值，跟獨立試算差距 X%」，而不是沉默地把可能失真
  的數字直接餵進 cost breaker 的門檻判斷。**交叉驗算邏輯本身還沒寫**，
  這份檔案目前只是定價數字的參考來源。

**C — 健壯性**
- **C5 receipt 宣稱機械交叉驗證（`files_changed`）**：supervisor（純
  bash）用 `git diff --stat` 核對 receipt frontmatter 的 `files_changed`
  清單跟該任務實際 commit 改到的檔案是否一致，對不上就計失敗——跟現有
  checkpoint mtime 交叉驗證同一個安全等級（純讀取已 commit 的 git
  狀態，零副作用、零權限繞道）。**明確排除**：不對 `tests.command`
  做機械重跑驗證，理由見 §4。

**V — 定位驗證**
- **V1 第二 agent 相容性驗證（Codex CLI）**：讓 Codex 接手一份既有的
  `.ai/` workspace 完成一個任務循環（讀 CONTRACT → 選任務 → 實作 →
  receipt → done.yaml → 印 AIOS_STATUS），**先後接手、不是平行**
  （single-writer 不動）。這是「agent-agnostic」從設計目標升級為
  已驗證主張的門檻；在此之前對外措辭一律是「協定除 skill 格式外
  不假設 Claude 專屬功能」。AI-RUNTIME 的「最小 agent 契約」節已先
  以規格形式寫出（六條 conformance 檢核）；Codex 實測後回填差距
  清單（哪些東西其實是 Claude 耦合：skill 載入方式、權限模型、
  /usage 解析）。
  **執行順序（fixture-first 的同一精神：先實測、後改碼）**：
  1. 零改動實測：測試 repo 手動 `codex exec`，把 /ai-work SKILL 演算法
     餵進 AGENTS.md，對照六條契約看哪裡斷（整檔重寫？狀態行？
     尊重 PAUSED？）→ 產出真實差距清單
  2. 憑清單改 supervisor，預定接縫是 schedule.yml 加 `agent_command`
     （預設 `claude -p "/ai-work" --output-format json`）——supervisor
     對 agent 的唯一要求：「在 repo cwd 跑一輪、結尾印 AIOS_STATUS
     行到 stdout」；cost 熔斷與 /usage quota 煞車解析不到就優雅
     降級（詞彙保留、策略停用），rate-limit 分類器先加 Codex 文案
     fixtures 再放寬。協定層 `.ai/` 檔案零改動。

**已完成（本輪稽核從「進行中」移除，2026-09-03）**：C1 rate-limit
偵測強化（fixture 先行＋`epoch` 精確睡眠形式，見 `supervisor.sh`
`sleep_until_reset`）、C2 狀態檔機械 lint（`lint_checkpoint`/
`lint_tasks`）、C4 allowlist 補洞（`Bash(date:*)`/`Bash(git show:*)`，
見 `templates/ai/settings.local.json`）、D1 `schedule-install.sh`
（commit `0438f3e`）、D2 `.ai/supervisor/events.jsonl`（commit
`e109ad0`，`event_type/status/metadata` 這類欄位切法可在未來擴充
loop 層事件時參考，範圍仍收斂在 supervisor 能機械觀察到的事，見
§4 新增條目）。都已反映進 §1 對照表。

## 4. 刻意不做

延續 README.md「Positioning」一節的立場——這是協定層，不是 agent
runtime，以下項目不在範圍內：

- **平行寫入者**：會打破 single-writer invariant，checkpoint/resume
  的可信度建立在「任何時刻只有一個 session 改 `.ai/`」上。
- **messaging gateways（Telegram/Discord/Slack…）**：那是通用助理
  框架的問題，不是「讓 coding agent 在這個 repo 裡負責任地工作」的
  問題。
- **model routing**：委派給 Claude Code（`claude -p "/ai-work"`），本
  repo 不重造 agent loop 或 model 選擇邏輯。
- **tool 生態系**：同上，工具執行是 Claude Code 的職責。
- **PreToolUse/PostToolUse hook 強制層**：目前的強制手段是
  permission deny 規則（`.claude/settings.local.json`）——它是
  best-effort 而非沙箱（誠實條款 1），但多一層版本耦合的 hook
  換不到質變，只是重複建置。
- **LLM 寫事件檔**：事件必須是機械發出（supervisor shell 直接寫
  `events.jsonl`），LLM 產生結構化日誌不可靠，這正是「已知限制 2」
  要避免的錯誤重演。
- **機械重跑 receipt 宣稱驗證（如重跑 `tests.command` 核對測試結果）**：
  評估過、明確否決，理由有四層：①成本——測試套件重跑一次，收尾時間
  加倍，長的整合測試會直接吃光 watchdog 預算；②不確定性——flaky test
  重跑可能失敗，會把系統雜訊誤判成 receipt 造假，錯誤地計進
  `consecutive_failures`；③環境不對等——`claude -p` session 有自己的
  venv/env vars，supervisor 外層 bash 不一定有同樣環境，重跑本身就可能
  因環境差異而失敗；④**最根本的**——這會繞過 `.claude/settings.local.json`
  的 allow/deny，讓 supervisor 直接執行 agent 寫進 receipt frontmatter
  裡的任意字串，開一條沒有權限保護的新執行路徑，跟「PreToolUse/
  PostToolUse hook 強制層」被拒絕的理由同一個精神：多一層執行路徑換不到
  對等的信任提升，只是多一個攻擊面。證據驗證交給 receipt 的
  `## 證據` 段落（要求貼實際測試輸出）＋ review 輪的 LLM 判斷力去決定
  要不要重跑——只有有判斷力的執行者才該做這個決定，機械腳本不該替它決定。
  純讀取、無副作用的機械交叉驗證（如 C5 的 `git diff --stat`）不受此限。
- **log rotation**：`.ai/supervisor/` 是 gitignored 執行狀態，量小
  且非長期資產，不值得引入 rotation 邏輯。
- **cron/視窗模式排程**：用作業系統原生的 launchd（D1），不用 cron
  或自製排程視窗——沒理由重造作業系統已經穩定提供的東西。
- **真 YAML parser 依賴**：`tasks/*.yaml` 走「整檔重寫＋壞檔自癒」
  策略（AI-RUNTIME.md checkpoint 規則），刻意不引入外部 parser 依賴
  以保持 supervisor 是純 shell、零安裝依賴。
- **session 內部 execution trace（tool-level／llm-level 事件，例如
  `tool.called`、`llm.started`）**：跟「LLM 寫事件檔」同一類但範圍更大
  的否決——`/ai-work` 對 supervisor 是一次不透明的 `claude -p` 黑盒呼叫，
  沒有機制能機械觀察到內部的 tool/LLM 呼叫；要拿到這層資料只能靠 agent
  自報（不可靠，就是「LLM 寫事件檔」被拒絕的理由）或耦合到特定 agent
  CLI 的 hook 機制（例如 Claude Code 的 PreToolUse/PostToolUse），兩者
  都跟「agent-agnostic」的既定方向相反（見 README「What's replaceable」、
  本文件最小 agent 契約），也跟 README positioning 明講的「not an agent
  framework, workflow engine, planner, or tool-calling layer」矛盾。
  這層留給 agent CLI 自己的原生功能演化（見 §5 最大風險），本協定只管
  loop 層與稽核層。（2026-09-03 一份外部 Observability 提案主張把這層
  變成 runtime 一級概念，評估後否決，理由同上。）

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
