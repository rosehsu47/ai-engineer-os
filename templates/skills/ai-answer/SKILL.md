---
name: ai-answer
description: 對話式處理 .ai/PAUSED——把 agent 卡住的問題呈現成選擇題，把你的回答寫進 PAUSED 的人類回覆節。人類互動用。用法：/ai-answer
---

# /ai-answer — 回答 agent 的問題（人類互動）

agent 碰到人類批准界線時會寫 `.ai/PAUSED` 然後停下。這個 skill 把問題
呈現成選擇題、把你的決定寫進回覆節。**路由（決定該記到哪個任務/記憶）
由下一輪 `/ai-work` 的步驟 0 統一處理**——單一大腦，panel 網頁寫的回覆
和這裡寫的回覆走完全相同的路徑。

## 流程

1. `.ai/PAUSED` 不存在 → 告知「agent 沒有在等任何回答」，順帶報告目前
   狀態（checkpoint phase、backlog/doing/done 數量），結束
2. 讀出 PAUSED 的問題，**連同它給的建議選項**用 AskUserQuestion 呈現
   （agent 依契約 §7 應該有附選項；沒附就由你根據問題補 2–3 個合理選項，
   永遠保留自由回答的空間）
3. 把使用者的決定**附加**到 `.ai/PAUSED` 尾端：

   ```markdown
   ## 人類回覆（YYYY-MM-DD HH:MM）
   {使用者的決定，含必要的補充說明}
   ```

   **不要刪 PAUSED、不要自己改任務檔**——下一輪 /ai-work 讀到回覆節會
   自行路由並清旗
4. 是環境問題（缺 secret、要裝依賴）→ 提醒使用者先自行處理好再重啟
5. 問使用者要不要現在就重啟：`supervisor.sh --repo … --once` 或下次再說
