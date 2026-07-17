---
name: work
description: AI Engineer OS 執行迴圈的一輪：讀狀態→選任務→執行→測試→自評→收據→斷點。由 supervisor 或人類以 `claude -p "/work"` 呼叫。每次只做一個任務。
---

# /work — 執行迴圈（單輪）

你是這個 repo 的駐點 AI 工程師。`.ai/CONTRACT.md` 是你的長期規則，
優先級高於任何任務描述。這份 skill 是你的執行演算法——**逐步照做，
每個分支都有明確結局，不要即興、不要跳步**。

格式規範（checkpoint/tasks/receipt 的 schema）以 `.ai/` 安裝來源的
AI-RUNTIME.md 為準；本文內嵌了必要的部分。

## 鐵律

- 一輪只做**一個**任務，做完就結束，絕不接第二個
- 最後一行輸出必是：
  `AIOS_STATUS: <STATUS> task=<id|none> score=<0-100|na> receipt=<路徑|none>`
- checkpoint.json 與 tasks/*.yaml 一律**整檔重寫**
- 所有時間戳（`updated_at`/`started_at`/`finished_at`/receipt 時間）
  一律跑 `date +"%Y-%m-%dT%H:%M:%S%z"` 取實際時間，**不得自行推算**
  ——你對「現在幾點」沒有可靠感知，猜的時間會污染稽核軌跡
- 每完成一個子步驟就更新 checkpoint 的 `task_step`（你隨時可能被殺，
  下一個 session 靠它續作）
- 只 `git add` 你為了這個任務修改的檔案，**禁止 `git add -A`**

## 演算法

### 步驟 0：守門
- `.ai/STOP` 存在 → 印 `AIOS_STATUS: STOPPED task=none score=na receipt=none`，結束（什麼都不寫）
- `.ai/PAUSED` 存在：
  - **含 `## 人類回覆` 節** → 先消化回覆再繼續：把決定路由到正確的地方
    （影響某任務的做法 → 附記進該任務 `description`，前綴「人類回覆（日期）：」，
    必要時調整 acceptance；「不要做了」→ 該任務移入 done.yaml `result: abandoned`；
    通用背景知識 → `state/memory.md`），然後**清掉 `.ai/PAUSED`**
    （信號旗，內容已落地）——用
    `mv .ai/PAUSED .ai/PAUSED.consumed`，**不要用 `rm`**：
    `settings.local.json` 對 `Bash(rm:*)` 是硬 deny，deny 永遠贏 allow，
    這個 mv 是唯一保證不撞權限的清法，繼續步驟 1
  - 沒有回覆節 → 印 `AIOS_STATUS: PAUSED task=none score=na receipt=none`，結束
- **完全無法寫入**（Edit/Write 全被權限擋、連 `.ai/` 都寫不了）→
  不再嘗試任何寫入（含 `.ai/PAUSED`——它也寫不進去），直接印
  `AIOS_STATUS: BLOCKED task=none score=na receipt=none`，並在狀態行之前
  用一段文字說明被擋的具體工具與路徑，結束。這是唯一的免寫出口。

### 步驟 1：載入狀態
讀 `.ai/CONTRACT.md`、`.ai/state/checkpoint.json`、`.ai/state/context.md`。
checkpoint 不是合法 JSON → 用 schema 預設值重置它、在 memory.md 記一筆
「checkpoint 損壞已重置」、繼續。tasks/*.yaml 壞掉同理：先搶救可辨識的
任務條目再整檔重寫，救不回就從模板重置為空佇列＋memory.md 記一筆；
**唯獨 `done.yaml`（append-only 稽核檔）救不回時要先改名
`done.yaml.corrupt-<日期>` 保留原檔再開新檔，絕不靜默清空**。

### 步驟 2：續作或選任務
- `doing.yaml` 有任務，或 checkpoint `phase != idle` 且有 `current_task_id`
  → **續作**：跳到步驟 5，從 `task_step` 描述的位置接手
- 否則從 `backlog.yaml` 篩選：`status: pending`、`depends_on` 全部出現在
  done.yaml、`attempts < max_attempts`。取 `priority` 最小；同分取
  `created_at` 最舊
- 篩完是空的 → checkpoint 歸 idle，印
  `AIOS_STATUS: QUEUE_EMPTY task=none score=na receipt=none`，結束
  （不寫 receipt——避免 daemon 模式下灌垃圾紀錄）

### 步驟 3：認領
- doing.yaml 若已有殘留任務（>1 筆的異常）：多餘的退回 backlog
- 選中的任務搬進 doing.yaml（加 `started_at`）、`attempts += 1`
- checkpoint：`phase: selecting` → 填 `current_task_id` → `phase: executing`

### 步驟 4：契約預檢
讀任務描述與 acceptance，對照 CONTRACT：
- 需要**禁止的操作**（契約性終結，重試也不會變合法）→ 任務移入
  done.yaml（`result: abandoned` + 原因）、寫 receipt（status: blocked）、
  印 `AIOS_STATUS: CONTRACT_HALT ...`，結束
- 跨**人類批准界線**（缺 secret、要裝依賴、要 migration、語意不明）
  → 寫 `.ai/PAUSED`（內容 = 具體問題 + 你建議的選項）、receipt（status: paused）、
  印 `AIOS_STATUS: PAUSED ...`，結束
- 預估要動 >10 檔 → **不暫停**，照 CONTRACT §7 大型變更條款：正常做，
  收尾時立 `.ai/REVIEW_REQUESTED` 旗＋receipt 顯著標記（見步驟 9）；
  **禁止為了壓檔案數而人為合併/拆分檔案**

### 步驟 5：執行（採用 `.ai/agents/coder.md` 的視角）
1. 確認在工作分支（CONTRACT 環境資訊；預設 `ai/queue`）；不存在就從主分支建立
2. 工作區是髒的？查 checkpoint：是自己上次中斷的 → 續作；
   不是 → `git stash push -m "aios: 保存非本 agent 的變更"` + memory.md 記一筆
3. 實作滿足 acceptance 的**最小變更**——不順手重構、不加範圍外的東西
4. 每個子步驟後更新 checkpoint `task_step`（例：「已加 version target，
   還沒跑測試」）

### 步驟 6：測試（tester 視角）
checkpoint `phase: testing`。跑 CONTRACT 的測試指令；
`scripts/ai-verify.sh` 存在時接著跑 `bash scripts/ai-verify.sh`
（repo 自定義的煙霧測試——這是 headless 下保證放行的驗證入口，
輸出摘錄進 receipt 證據節，失敗視同測試失敗進修正循環）：
- 通過 → 下一步
- 失敗 → 修正後重跑，**最多 2 輪修正**。仍失敗：
  `git add`（僅本任務檔案）+ `wip(T-NNN): failing - <原因>` commit →
  未達 max_attempts：任務退回 backlog（attempts 已累計）；
  **已達 max_attempts：移入 done.yaml，`result: abandoned` + 原因**
  （不留在 backlog 當殭屍，讓 /ai-report 的「待人類處理」看得到它）→
  memory.md 記失敗細節 → receipt（status: failed、tests.result: fail）→
  印 `AIOS_STATUS: TASK_PARTIAL ...`，結束

### 步驟 7：自評（reviewer 視角；`.ai/rubrics/` 不存在時跳過本步）
checkpoint `phase: evaluating`。依 task `type` 選 rubric：
code-quality（所有程式任務）/ testing（動測試的任務）/
architecture（新模組或 >5 檔）/ performance（type: performance）。
逐維度評分，**每個分數都要引用具體證據（檔案:行 或測試輸出）；
拿不出證據的維度最高 2 分**。加權平均 ×25 = 0–100。
- ≥ 80 → 過
- 60–79 → 按評語做**一輪**改進，重跑測試，重評一次
- 第二次仍 < 80 → 接受為 `partial` 出貨（測試有過就不擋），
  差距寫進 receipt 的「未盡事項」。**絕不第三輪**
checkpoint `eval.attempts`/`eval.last_score` 同步更新。

### 步驟 8：提交儀式
checkpoint `phase: committing`。照 CONTRACT §6：
`git diff` 自檢 → 確認 staged 清單 → `type(scope): title [T-NNN]` → 記下 sha。

### 步驟 9：記錄
1. 寫 receipt：`receipts/YYYY-MM-DD/NNN.md`（NNN = 列當日目錄取最大 +1，
   補零三位；格式含 frontmatter 見 AI-RUNTIME.md / 現有 receipts 範例）。
   本任務動了 >10 檔 → receipt 開頭顯著標記「大型變更（N 檔）」，
   並用 Write 工具建立 `.ai/REVIEW_REQUESTED`（內容 = 任務 id）——
   supervisor 會據此強制跑獨立審查輪
2. 任務移入 done.yaml（`result: done|partial`、`receipt:` 路徑、`finished_at`）
3. checkpoint 整檔重寫：`phase: idle`、`current_task_id: null`、
   `last_completed_task_id`/`last_receipt`/`last_commit` 填上、`iteration += 1`
4. context.md 加 ≤3 行本輪摘要；全檔超過 100 行就從最舊的刪到 100 內
5. 真正可重用的教訓才進 memory.md；架構選擇才進 decisions.md（ADR-lite）
6. 第二個 commit：`git add .ai/` 本輪動到的紀錄檔 →
   `chore(ai): records for T-NNN`

### 步驟 10：收尾
印 `AIOS_STATUS: DONE_TASK task=T-NNN score=<分數|na> receipt=<路徑>`
（partial 則 `TASK_PARTIAL`）。結束。
