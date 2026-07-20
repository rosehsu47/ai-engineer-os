---
name: ai-answer
description: 掃描所有已註冊 repo 的 .ai/PAUSED，列出所有待回答問題，逐一用選擇題引導回答並寫入回覆節。用法：/ai-answer [repo路徑 ...]
---

# /ai-answer — 一次處理所有 repo 的待答問題（人類互動）

跨 repo 版本；單一 repo 內建的版本在 `templates/skills/ai-answer/SKILL.md`
（裝進目標 repo 後用同名指令，只看那個 repo）。這支從 **這裡**（`ai-engineer-os`）
執行，掃過你所有在跑 supervisor 的 repo，一次把問題都攤開來問完。

## 步驟

### 1. 決定要掃描的 repo 清單

- 使用者有給參數（一或多個 repo 絕對路徑）→ 只掃這些
- 沒給參數 → 讀 `~/.aios-repos`（一行一個路徑，`#` 開頭是註解，跳過空行）
  - 檔案不存在或清單為空 → 告知使用者「找不到 repo 清單，用
    `/ai-answer {repo路徑}` 指定，或建立 `~/.aios-repos`（格式見
    `panel/README.md`）」，結束

對每個路徑先確認 `{repo}/.ai/` 存在（不是就跳過並在最後總結時註明「未
`/ai-init`，略過」，不要中斷整體流程）。

### 2. 掃描各 repo 的 `.ai/PAUSED`

對每個有效 repo：
- 不存在 `.ai/PAUSED` → 乾淨，不列入
- 存在但已含 `## 人類回覆` 節 → **已回覆、等消化**（回覆已經寫了，只是
  還沒有 supervisor 輪次讀過它）；列入總結但不要重複問
- 存在且不含 `## 人類回覆` 節 → **待回答**

### 3. 沒有任何「待回答」項目

報告目前狀態：哪些 repo 乾淨、哪些是「已回覆、等消化」。對後者，問使用者
要不要現在就對這些 repo 各跑一輪
`supervisor/supervisor.sh --repo {repo} --once`
（每個獨立問一次或用 multiSelect 一次問完都可以）。結束。

### 4. 有「待回答」項目

1. 先列出總覽（一個表或條列）：repo 名稱、任務 id、問題一行摘要
2. **逐一**處理每個待回答項目（不要一次全塞進一個問題，agent 給的選項
   彼此獨立、混在一起會讓使用者選錯對象）：
   - 讀出該 `.ai/PAUSED` 的「問題」與「建議選項」節
   - 用 AskUserQuestion 呈現：選項照抄 agent 建議的（保留其語意），
     agent 沒附選項就依問題內容自己補 2–3 個合理選項——**永遠**保留
     使用者能自由輸入其他答案的空間（AskUserQuestion 本身就有 Other）
   - 把使用者的決定**附加**到該 `.ai/PAUSED` 尾端：
     ```markdown
     ## 人類回覆（YYYY-MM-DD HH:MM）
     {使用者的決定，含必要的補充說明}
     ```
   - **不要刪 `.ai/PAUSED`、不要自己改任務檔／backlog.yaml**——下一輪
     `/ai-work` 讀到回覆節會自行路由並清旗，這是唯一的寫入路徑
   - 是環境問題（缺 secret、要裝依賴、要 schema migration）→ 額外提醒
     使用者：這類問題就算寫了回覆，agent 多半還是需要你先在 repo 外
     把環境準備好（例如把 secret 塞進 `.env`），寫回覆不會自動生出憑證

### 5. 收尾

全部處理完後：
- 總結這次寫了幾個 repo 的回覆
- 問使用者要不要現在就對每個「剛寫入回覆」的 repo 各跑一輪
  `supervisor/supervisor.sh --repo {repo} --once`（多個 repo 可以
  multiSelect 一次選，或依序問）——這樣回覆會立刻被下一輪 `/ai-work` 消化
  並清掉 `.ai/PAUSED`；使用者也可以選之後自己再跑
