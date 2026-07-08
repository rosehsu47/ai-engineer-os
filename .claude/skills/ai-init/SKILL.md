---
name: ai-init
description: 在目標 repo 安裝 .ai/ Agent Runtime Workspace（複製模板、訪談填契約、設定權限）。用法：/ai-init {目標 repo 絕對路徑}
---

# /ai-init — 安裝 AI Runtime Workspace

把 `work-record-tool/templates/ai/` 安裝進目標 repo，訪談使用者填 CONTRACT，
設定 headless 執行所需的權限。**照步驟做，不要即興。**

## 步驟

1. **前置檢查**
   - 參數必須是絕對路徑且是 git repo（`git -C {path} rev-parse` 成功）；否則說明並停止
   - `{path}/.ai/` 已存在 → 告知使用者並停止（要重裝請使用者自行先搬走舊的；
     絕不覆蓋既有的 runtime 狀態）

2. **複製模板**
   - 複製 `work-record-tool/templates/ai/` 整棵 → `{path}/.ai/`
     （`settings.local.json` 除外——它屬於 `.claude/`，見步驟 4）
   - 複製 `work-record-tool/templates/skills/work/SKILL.md` →
     `{path}/.claude/skills/work/SKILL.md`（**skill 必須住在目標 repo**——
     `claude -p "/work"` 在目標 repo 執行時只看得到那裡的 skills；
     deny 規則同時保證 agent 不能改寫自己的迴圈）
   - `mkdir -p {path}/.ai/supervisor`

3. **訪談**（一次問完，AskUserQuestion 或條列提問皆可）：
   1. 這個 agent 的使命是什麼？（一兩句；負責什麼、不負責什麼）
   2. 測試指令？（如 `make test`、`npm test`；沒有測試就填建置指令並註明）
   3. 建置指令？
   4. 主分支名？（預設用 `git -C {path} symbolic-ref --short HEAD` 的現值）
   5. repo 特有的禁止事項？（如「不修改 migrations/ 既有檔案」；可空）
   6. `.ai/` 要 commit 進 git 嗎？（預設要——審計紀錄的價值就在可追溯；
      選不要就把 `.ai/` 整個加進 .gitignore）

4. **填契約與權限**
   - 把訪談結果填進 `{path}/.ai/CONTRACT.md` 的所有 `{{...}}` 佔位符
     （`{{EXTRA_FORBIDDEN}}` 沒有就整行刪掉）
   - 讀模板 `settings.local.json`，替換 `{{TEST_COMMAND}}`/`{{BUILD_COMMAND}}`
     為指令的**第一個詞**（如 `make test` → `Bash(make test:*)` 直接用全指令），
     移除 `_comment`，**併入** `{path}/.claude/settings.local.json`：
     - 目標檔不存在 → 直接寫入
     - 已存在 → 合併 allow/deny 陣列（去重、保留原有條目），不動其他 key
   - `{path}/.gitignore` 加一行 `.ai/supervisor/`（已有就跳過）

5. **驗證與收尾**
   - Read-back：`.ai/` 樹完整（CONTRACT/schedule.yml/tasks×3/state/checkpoint.json/
     rubrics/agents/receipts/reports）、CONTRACT 沒有殘留 `{{`、
     settings 合併後是合法 JSON
   - 問使用者要不要現在種第一個任務；要就照 AI-RUNTIME.md 的 backlog schema
     寫進 `backlog.yaml`（提醒 acceptance 要可客觀驗證）
   - Commit（在目標 repo，**只 add 這次建立/修改的檔案**）：
     `chore(ai): initialize AI runtime workspace`
   - 告訴使用者下一步：`claude -p "/work"` 手動跑一輪，或
     `supervisor/supervisor.sh --repo {path} --once`
