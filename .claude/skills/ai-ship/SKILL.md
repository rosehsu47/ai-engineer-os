---
name: ai-ship
description: 把目標 repo 的 ai/queue 分支推上 GitHub 並開/更新 PR（描述由 receipts 自動生成）。對外操作，僅限人類觸發。用法：/ai-ship {repo路徑}
---

# /ai-ship — 出貨到 GitHub（人類觸發）

把 agent 在 `ai/queue` 分支累積的工作推上 GitHub 並開 PR。
**這是對外動作**：supervisor 永不自動呼叫它；agent 的 settings deny 也擋著
`git push`——推送發生在你（人類）的這個 session，由你的權限批准。

## 前置檢查（任一不符就說明並停止）

1. `{repo}` 是 git repo、`ai/queue` 分支存在且領先主分支至少 1 個 commit
2. `gh` CLI 可用且已登入（`gh auth status`）；沒有 gh → 給出手動指令
   （`git push -u origin ai/queue` + GitHub 網頁開 PR）後停止
3. repo 有 origin remote

## 步驟

1. 產生 PR 描述：依 `/ai-report` 的規則，把 `ai/queue` 上所有
   `[T-NNN]` commit 對應的 receipts 彙整成「做了什麼/為什麼/怎麼驗證」
   三節 + 任務表（含 rubric 分數與獨立審查判定）。
   結尾加一行：`🤖 Generated with AI Engineer OS (work-record-tool)`
2. 給使用者看 PR 標題與描述草稿，**確認後**才執行：
   `git push -u origin ai/queue`
3. 已有 open PR（`gh pr list --head ai/queue`）→ `gh pr edit` 更新描述；
   沒有 → `gh pr create --base {主分支} --head ai/queue`
4. 回報 PR URL。**不合併**——merge 永遠是人類的決定。
