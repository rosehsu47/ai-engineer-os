---
name: ai-answer
description: 對話式處理 .ai/PAUSED——把 agent 卡住的問題呈現成選擇題，記錄你的決定，解除暫停。人類互動用。用法：/ai-answer
---

# /ai-answer — 回答 agent 的問題（人類互動）

agent 碰到人類批准界線時會寫 `.ai/PAUSED` 然後停下。這個 skill 把
「讀檔→思考→改任務→刪檔」的手工流程變成一次對話。

## 流程

1. `.ai/PAUSED` 不存在 → 告知「agent 沒有在等任何回答」，順帶報告目前
   狀態（checkpoint phase、backlog/doing/done 數量），結束
2. 讀出 PAUSED 的問題，**連同它給的建議選項**用 AskUserQuestion 呈現
   （agent 依契約 §7 應該有附選項；沒附就由你根據問題補 2–3 個合理選項，
   永遠保留自由回答的空間）
3. 把使用者的決定落到正確的地方：
   - 影響某個任務的做法 → 把決定**附記進該任務的 `description`**
     （前綴「人類回覆（日期）：」），必要時調整 acceptance
   - 是「不要做了」→ 該任務移入 done.yaml（`result: abandoned` + 原因）
   - 是環境問題（缺 secret、要裝依賴）→ 提醒使用者先自行處理好，
     確認處理完才進下一步
   - 通用背景知識 → 記進 `state/memory.md`
4. 給使用者複述一次「我記錄了什麼、agent 下一輪會怎麼理解」，確認後
   **刪除 `.ai/PAUSED`**（這是唯一允許刪除的協定檔——它是信號旗不是紀錄；
   問答的內容已落在任務/記憶裡）
5. 問使用者要不要現在就重啟：`supervisor.sh --repo … --once` 或下次再說
