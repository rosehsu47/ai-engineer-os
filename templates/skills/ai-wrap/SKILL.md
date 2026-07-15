---
name: ai-wrap
description: 互動 session 收帳——把這輪對話直接改的程式碼收成 commit + receipt（source: human-interactive）+ done.yaml 條目，讓 panel 與 /ai-report 看得到。人類互動用。用法：/ai-wrap（可附一句這輪做了什麼）
---

# /ai-wrap — 互動 session 收帳（人類互動）

互動對話（人類貼截圖、即時反饋、Claude 直接改）不走 `/work` 迴圈，
但**走同一條稽核軌道**：程式碼要 commit、工作要有 receipt。這個 skill
把收尾三步收成一個指令。原則：**如實記錄，不美化**——互動 session 沒有
預定義 acceptance，就誠實寫 `rubric: null`，不要事後編造驗收條件。

## 流程

1. **盤點**：`git status --porcelain` 列出未 commit 的變更。
   - 全空 → 檢查是否有「已 commit 但沒 receipt」的情況（最近的
     `[T-NNN]` commit 是否在 done.yaml 有對應條目）；都齊了就告知
     「無帳可收」並結束
   - **只收本輪對話實際動過的檔案**——工作區可能有人類自己的 WIP，
     不確定哪些是本輪的就逐檔問使用者，絕不 `git add -A`
2. **一句話訪談**（使用者已隨指令附上就不再問）：這輪做了什麼？
   為什麼？（一兩句即可，細節從對話上下文與 diff 補齊）
3. **驗證**：跑 CONTRACT 環境資訊裡的測試指令（沒有就建置指令）。
   失敗 → 告知使用者並停，**不 commit 壞的程式碼**；使用者明確說
   「先收」才繼續（receipt `tests.result` 如實標 fail）
4. **取號**：任務 id 取 done.yaml + backlog.yaml + doing.yaml 現有最大
   T-NNN +1；receipt 編號照常（當日目錄最大流水號 +1，補零三位）
5. **寫帳**（三份，缺一不可）：
   - 程式碼 commit：`type(scope): 摘要 [T-NNN]`（只 add 盤點確認的檔案）
   - Receipt：AI-RUNTIME.md 格式，`source: human-interactive`、
     `rubric` 全 null；「做了什麼」按時間序寫、「證據」附測試輸出摘錄
     與 `git diff --stat`；所有時間戳（`finished_at`、frontmatter 時間）
     跑 `date +"%Y-%m-%dT%H:%M:%S%z"` 取實際時間，不得自行推算
   - done.yaml 尾端 append 條目（含 `source: human-interactive`）
   - receipt + done.yaml 併入同一個 commit 或緊接著
     `chore(ai): records for T-NNN`——跟 /work 的雙 commit 慣例一致
6. **收尾**：回報 commit sha 與 receipt 路徑。panel 下次刷新（5 秒）
   就會顯示這筆。

## 邊界

- 互動 session 人類在場即人類批准：可以在人類指定的任何分支收帳
  （含主分支），不受 agent 的 `ai/queue` 限制
- `.ai/PAUSED`/`.ai/STOP` 存在時照常運作——它們管的是無人迴圈，
  不是人類互動
- 這個 skill 只「記帳」，不「補做」：發現沒做完的事，提議用
  `/ai-task` 種進 backlog，不要在收帳時順手擴大範圍
