---
name: ai-task
description: 用問答/選擇題引導的方式新增任務到 .ai/tasks/backlog.yaml，不用手寫 YAML。人類互動用。用法：/ai-task（可直接附上任務描述）
---

# /ai-task — 引導式種任務（人類互動）

把「我想要 agent 做什麼」變成一筆合格的 backlog 任務。**全程對話引導，
能用選擇題就用選擇題（AskUserQuestion），不逼使用者手寫 YAML。**

## 流程

0. **檢查兩層鎖**：讀 `.ai/supervisor/lock`（supervisor.sh 整個無人迴圈
   期間持有）與 `.ai/state/session.lock`（單次 `/ai-work`／`/ai-review`
   呼叫期間持有），各自存在就取其中的 pid、用 `kill -0 <pid>` 確認還活
   著。任一個活著都代表有 session 正在這個 repo 跑，隨時可能整檔重寫
   `backlog.yaml`——這個 skill 最後一步也是整檔重寫，兩邊同時寫會互相
   蓋掉（單一寫入者不變量，見 AI-RUNTIME.md）。用 AskUserQuestion 讓使
   用者選：等它跑完再種任務（推薦）／仍要現在寫入（講清楚有覆寫風
   險）。兩份 lock 都不存在或 pid 都已死就正常往下走。
1. **聽需求**：使用者沒附描述就先問「想讓 agent 做什麼？」（自由文字）
2. **起草**：根據描述起草整筆任務，然後用選擇題確認三件事：
   - `type`：feature / fix / chore / test / docs / architecture / performance
     （附一句你判斷的理由，預設選項放最前）
   - `priority`：1 立刻做 / 2 正常 / 3 有空再說
   - `acceptance`：**你起草 2–3 條可客觀驗證的條件給使用者挑選/修改**——
     這是最關鍵的一步，「可驗證」的範例：「跑 X 指令輸出 Y」「頁面 Z 出現
     某元素」「測試 T 通過」；不合格的範例：「程式碼品質良好」
3. **拆量檢查**：如果這個任務一個 session 做不完（經驗值：要動 >5 個檔案
   或有多個獨立的 acceptance 群），建議拆成多筆 + `depends_on` 串接，
   問使用者要不要拆
4. **預覽**：把完整 YAML 條目秀給使用者看（id 取現有最大 T-NNN +1，
   `created_at` 用現在時間），確認後寫入 `.ai/tasks/backlog.yaml`
   （整檔重寫，保留既有任務）
5. **收尾**：告訴使用者下一步——
   `claude -p "/ai-work"` 跑一輪，或 supervisor 會在下輪自動接手
