---
name: ai-review
description: AI Engineer OS 的獨立審查輪：用全新 session 審查上一個完成的任務（fresh context，非實作者自評）。由 supervisor 在 DONE_TASK 後呼叫，或人類手動 `claude -p "/ai-review"`。
---

# /ai-review — 獨立審查（多 agent 的第二雙眼睛）

你是**沒有參與實作**的審查者。實作 session 已經自評過（rubric），你的價值
是全新視角：實作者看不到自己的盲點。**只審查上一個完成的任務，
發現問題不自己修**——修正工作開成新任務排隊，維持單一寫手不變量。

## 鐵律

- 你只能寫這三個地方：受審 receipt 的尾端（附加審查節）、
  `tasks/backlog.yaml`（開修正任務）、`state/context.md`（≤2 行）
- **絕不修改程式碼**、絕不 commit 程式碼、絕不動其他任務
- 最後一行輸出必是：
  `AIOS_REVIEW: <PASS|FAIL> task=<id> followup=<新任務id|none>`

## 步驟

1. 讀 `.ai/state/checkpoint.json` 的 `last_completed_task_id` 與
   `last_receipt`、`last_commit`。任一為空 → 印
   `AIOS_REVIEW: PASS task=none followup=none`（沒東西可審），結束
2. 讀該 receipt、`git show <last_commit>` 的完整 diff、
   對應任務在 done.yaml 的 acceptance
3. 用 `.ai/rubrics/` 中對應的量表**重新獨立評分**（不看實作 session 給的
   分數先評，評完再對照）；另外專查實作者自評最容易漏的三件事：
   - acceptance 是否真的全部滿足（逐條對 diff 驗證，不信 receipt 的宣稱）
   - 範圍外改動（diff 裡有沒有任務不需要的變更）
   - 測試是否真的驗到新行為（不是只有既有測試碰巧通過）
4. 判定：
   - 獨立評分 ≥80 且三項專查無問題 → **PASS**
   - 否則 → **FAIL**：在 backlog.yaml 開一個修正任務
     （`id` 取下一個 T-NNN、`priority: 1`、`type: fix`、
     description 引用原任務與你的具體發現、acceptance 寫可驗證的修正條件、
     `depends_on: []`），follow-up 任務會被下一輪 /ai-work 接走
5. 在受審 receipt 尾端附加：

   ```markdown
   ## 獨立審查（/ai-review，fresh session）
   - 判定：PASS|FAIL
   - 獨立評分：NN（實作自評：NN）
   - 發現：（無，或條列；FAIL 時註明 follow-up 任務 id）
   ```

6. `context.md` 加 ≤2 行審查摘要，印 `AIOS_REVIEW:` 行，結束
