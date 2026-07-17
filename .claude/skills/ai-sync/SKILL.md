---
name: ai-sync
description: 把已 /ai-init 的 repo 同步到最新模板：skills 自動補齊、settings 漂移列 diff 待確認、驗證腳本補裝。用法：/ai-sync [repo路徑 ...]
---

# /ai-sync — 模板演化後，把已裝的 repo 拉回同步（人類互動）

模板（`templates/`）每天在演化，已 `/ai-init` 的 repo 不會自動跟上——
實測一週內兩個 repo 都漂移（缺新 skill、缺新 allow 條目）。這支從
`ai-engineer-os` 執行，把差距分三類處理：**純複製的自動補、
人類控制面的列 diff 等確認、可選慣例的問過再裝**。

## 步驟

### 1. 決定要同步的 repo 清單

- 有參數（一或多個絕對路徑）→ 只同步這些
- 無參數 → 讀 `~/.aios-repos`（同 /ai-answer 的規則：`#` 註解、跳空行）
- 每個路徑確認 `{repo}/.ai/` 存在，不是就跳過並在總結註明「未 init，略過」

### 2. Skills 同步（自動——skills 是純複製，模板是唯一事實來源）

對 `templates/skills/` 下每個 skill（work/review/ai-task/ai-answer/ai-wrap）：
- 目標 repo 缺 → 直接複製，記入報告
- 目標 repo 有但內容與模板不同 → **直接以模板覆蓋**（CLAUDE.md 鐵律：
  修模板不修副本；repo 端的客製不該存在，若 diff 看起來像是有人刻意
  客製過，先展示 diff 問使用者再覆蓋）

### 3. Settings 漂移（人類控制面——只列不改，除非使用者確認）

比對 `templates/ai/settings.local.json` 與
`{repo}/.claude/settings.local.json`：
- 模板 allow/deny 有、目標缺的條目（跳過 `{{...}}` 佔位）→ 列成清單，
  用 AskUserQuestion 問使用者是否套用；同意才動手，逐 repo 處理
- 編輯被權限系統擋下時：印出精確的「要加哪幾行、加在哪」手動指引，
  不嘗試繞過（settings 是人類控制面，被擋是設計的一部分）
- 目標多出來的條目（repo 特有的測試/建置指令等）**不動也不報錯**——
  drift 檢查只管模板條目有沒有到齊

### 4. 驗證腳本（可選慣例——問過再裝）

`{repo}/scripts/ai-verify.sh` 不存在 → 問使用者要不要裝模板骨架
（順便說明用途：headless 下 agent 唯一保證放行的端到端驗證入口）。
已存在 → 不動（內容是 repo 自己的，永不覆蓋）。

### 5. 收尾

- 總結表：每個 repo 補了哪些 skills、settings 套用/待手動/略過、
  verify script 裝了沒
- 提醒：同步後跑 `supervisor.sh --doctor --repo {repo}` 確認全綠
  （doctor 的 drift 檢查就是本 skill 的驗收）
- 本 skill 永不碰：`.ai/` 狀態檔（tasks/checkpoint/receipts）、
  CONTRACT.md（契約是 repo 專屬的訪談產物，模板更新不回灌）、
  `schedule.yml`（人類調參，只在報告裡提示模板預設值有變）
