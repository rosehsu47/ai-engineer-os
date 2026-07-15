---
name: ai-report
description: 把目標 repo 的 .ai/receipts 彙整成日報/週報（含 PR 描述草稿、Changelog、履歷素材）。用法：/ai-report {repo路徑} {daily|weekly|YYYY-MM-DD..YYYY-MM-DD}
---

# /ai-report — 從 receipts 產工程報告

讀 `{repo}/.ai/receipts/` 的收據 frontmatter 與內文，彙整成一份報告。
**只用收據裡有的事實，不腦補**——這份報告的下游可能是履歷或工程週報，
證據紀律等同履歷。

## 範圍解析
- `daily` → 今天（無收據則取最近有收據的一天，並註明）
- `weekly` → 本週一至今
- `YYYY-MM-DD..YYYY-MM-DD` → 指定區間

## 輸出：`{repo}/.ai/reports/{daily-YYYY-MM-DD | weekly-YYYY-Www | range-...}.md`

依序包含六節：

1. **完成任務總覽**——表格，直接來自 frontmatter：
   `| 任務 | 標題 | 狀態 | 分數 | commit | 收據 |`
2. **變更與測試證據**——每個任務一小段：改了哪些檔（files_changed）、
   測試結果（tests）、收據「證據」節的關鍵摘錄
3. **PR 描述草稿**——把區間內 status=done|partial 的任務整合成一段
   可直接貼上 PR 的描述（做了什麼/為什麼/怎麼驗證），引用 commit sha
4. **Changelog 片段**——`- type: title [T-NNN]` 條列，可直接貼 CHANGELOG
5. **履歷素材**——照 CONVENTIONS.md 的證據格式輸出兩個表，
   讓 /new-project-intro 與 /new-resume 能直接搬列：
   - Talking points：`| Topic | Evidence | What to Say |`
     （Evidence 欄必附收據路徑）
   - Key Numbers：`| Metric | Value | 來源收據 |`
     （只收有量測證據的數字；沒有就明寫「本期無可引用數字」）
6. **運行事件摘要**——來源 `{repo}/.ai/supervisor/events.jsonl`
   （loop 層事件，schema 見 AI-RUNTIME.md 事件模型；**檔案不存在就
   整節略過**，不要編造）。只做區間內的計數與加總：迭代輪數、
   rate-limit 睡眠次數、quota 煞車/等待次數、watchdog kill 與 crash
   輪數、每日成本合計。這節是運行遙測，**不進成就敘述**——
   成就相關各節（1-5）的唯一事實來源仍是 receipts

## 規則
- 一個任務多張收據（失敗後重試）→ 以最後一張為準，前面的列為歷程
- `blocked/paused` 的任務放進「待人類處理」小節，不混進成就
- 報告開頭附統計行：任務數/完成/部分/失敗、rubric 平均分
