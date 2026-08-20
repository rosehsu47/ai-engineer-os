---
name: ai-healthcheck
description: 稽核目標 repo 的 CONTRACT.md 與 tasks（backlog/doing）內容本身，抓出「執行前就能判斷會導致交付失敗或漏東西」的結構性缺陷（窮舉型 acceptance 缺可核驗清單、DoD 與 headless 執行環境衝突、事實類 acceptance 缺依據來源、語意模糊、depends_on 圖問題）。純讀不寫，唯讀報告。用法：/ai-healthcheck {repo路徑}
---

# /ai-healthcheck — 交付前的內容健檢（人類互動）

`/ai-review` 抓的是「執行完之後，diff 有沒有滿足 acceptance」；這個 skill
抓的是更早一層——**任務被寫進 backlog 的那一刻，acceptance/CONTRACT 本身
有沒有結構性缺陷，讓 headless 執行注定漏東西或卡死**。跟框架健不健康
無關（那是 `supervisor.sh --doctor` 的事），只看**這個專案的業務內容**：
CONTRACT 契約條款、任務 acceptance 寫法、任務引用的 spec/來源文件。

**只唯讀分析，不動 backlog、不動 CONTRACT**——這裡抓到的都是需要人類
判斷「這句話原意是什麼」的內容問題，不是機械式能自動改的。

## 範圍

讀 `{repo}/.ai/CONTRACT.md`、`{repo}/.ai/tasks/backlog.yaml`、
`{repo}/.ai/tasks/doing.yaml`。**不含 `done.yaml`**——已完成的任務有
`/ai-review` 追殺，這個 skill 只管「還沒執行、來得及先改」的任務。

## 五類檢查

### 1. 窮舉型 acceptance 缺可核驗清單
逐條掃 acceptance 文字，命中「全部/每一個/不遺漏/涵蓋/所有」等窮舉語意
的字眼時：
- acceptance 或 description 有沒有指名一份具體、可逐項核對的來源
  （檔案 + 章節/欄位，例如「依 X.md 的 Y 清單逐項列出」）？
- 指名的來源檔案實際存在嗎（`ls`/`find` 確認）？
- 沒指名來源，或指名的檔案不存在 → **高風險**：這條 acceptance 沒有
  客觀核對基準，執行者跟自評都容易漏，只能等 fresh review session
  才會被抓到（多耗一輪）。列出任務 id + 命中的原句 + 建議補的來源指向。

### 2. DoD／CONTRACT 與 headless 執行環境衝突
- 掃 CONTRACT §5（DoD）與 §2（測試指令）有沒有「手動驗證」「瀏覽器」
  「GUI」「人工操作」等字眼。
- 若有，檢查 §7（人類批准界線）是不是已經有對應的 PAUSE 條款——
  沒有 → **高風險**：DoD 要求 headless session 做不到的事，且沒有
  PAUSE 出口，任務會不斷嘗試、不斷失敗，直到 `max_attempts` 才被標
  blocked，是隱性卡死，不是任務本身的邏輯問題。
- 逐條掃 backlog/doing 的 acceptance，命中「開啟瀏覽器」「點擊」
  「手動」「UI 上確認」等字眼、且該任務沒有依賴任何 human-interactive
  流程（`/ai-wrap`）→ **高風險**，列出任務 id + 原句，建議：要嘛把
  acceptance 改成 headless 可驗證的形式（靜態追蹤/log 輸出），要嘛
  明確標記需要人類互動 session 才能收尾。

### 3. 事實／數字類 acceptance 缺依據來源
針對 `type: fix` 且 description 出現「修正/更正/撤回/糾正」等字眼、
內容在改動先前對某個事實/數字/引用的認定的任務：
- acceptance 有沒有明確要求「附一句依據來源，指出根據哪個既有檔案/
  欄位判定」？沒有 → **中風險**：下一個執行 session 若照單全收上一輪
  的結論、不回頭核對原始來源，容易把本來對的內容改錯（AI-RUNTIME.md
  「修正引用/事實類的 followup 任務」條款已定義這個規則，這裡是檢查
  任務有沒有真的照規則寫）。

### 4. acceptance 語意模糊
逐條讀 acceptance，判斷「兩個人各自照字面做，會不會做出不一樣的東西」
（CONTRACT §7 定義 PAUSE 的同一個標準，這裡是在執行前先掃一輪）。
常見模式：用「品質良好」「大致完成」「合理」這類主觀詞收尾、沒給
可觀察的判定條件。命中 → **中風險**，列出任務 id + 原句 + 兩種可能的
合理解讀，供人類決定要不要先澄清。

### 5. depends_on 圖問題
解析 backlog/doing 全部任務的 `depends_on`：
- 指向不存在的 task id → **高風險**（任務永遠選不到，卡在 backlog）
- 循環依賴（A→B→A）→ **高風險**

## 輸出

`{repo}/.ai/reports/healthcheck-YYYY-MM-DD.md`：

1. 統計行：backlog+doing 任務數、五類各命中幾筆
2. 逐類列表：任務 id、標題、命中原句、風險等級（高/中）、建議修法
3. 若五類都零命中：明講「本輪無結構性缺陷」，不硬湊內容

**結尾不自動動手**——列完報告後問使用者要不要用 `/ai-task` 補救（例如
幫某條 acceptance 補上來源指向、幫 CONTRACT §7 加 PAUSE 條款），使用者
確認要修哪幾筆才動手，一次只處理使用者選中的項目。

## 規則
- 本 skill 只活在 `ai-engineer-os`，不安裝進目標 repo 的 `.claude/skills/`
  （跟 `/ai-report`/`/ai-sync`/`/ai-init` 同一組，只對外分析，不進
  headless 執行迴圈）
- 找不到 `{repo}/.ai/` → 提示先跑 `/ai-init`，結束
- 這個 skill 抓到的都是「這句話寫得夠不夠精確」的內容判斷，不是
  機械 lint——寧可少抓、附具體原句給人類判斷，不要為了湊數把正常的
  acceptance 也標成風險
