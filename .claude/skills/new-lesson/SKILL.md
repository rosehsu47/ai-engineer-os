---
name: new-lesson
description: Capture a project-specific gotcha into LESSONS.md — a quick-scan symptom-fingerprint entry for future sessions (human or AI). Use when the user corrects a wrong assumption Claude made, a bug turns out to be caused by misunderstanding this codebase's architecture, or a review catches a "should have known better" issue. Write it immediately, don't wait for wrap-up.
---

# new-lesson

> 把一個「這個專案特有、直覺會出錯」的教訓寫進 `LESSONS.md`。不是敘事、不做多輪
> Q&A，是快速記下四個欄位，讓未來的 session 一眼比對症狀就認出來。

---

## 何時使用

- 使用者糾正了 Claude 的一個錯誤假設（「不是這樣的，其實…」）
- 追出一個 bug，發現根因是誤解了架構（誤以為某個 pattern 存在、誤以為某個 flow 是這樣運作）
- Code review 抓到一個「早知道就不會犯」的問題
- `/new-lesson`

**不要用於**：
- 一般性最佳實踐、跟這個專案無關的通則 → 那是 `CLAUDE.md` 的事
- 單純的 bug 修復、沒有「誤解」成分 → PR description 寫起因/根因/修法/影響範圍就夠，不需要進
  `LESSONS.md`
- 策略性思考、產品方向、還沒想清楚的取捨 → 用其他 journal 類 skill（若專案有的話）

如果不確定這件事夠不夠格進 `LESSONS.md`，直接問使用者，不要自己硬寫。

---

## 流程

### Step 1 — 確認觸發情境

檢查這件事是不是真的「這個專案特有、直覺會出錯」：
- 有沒有具體、可驗證的症狀（錯誤訊息、行為、檔案位置）？不是模糊的「要注意 XX」
- 根因是不是「誤解」造成的（架構、既有 pattern、資料流），而不是單純打字打錯或漏測？

兩者都成立才繼續。不成立的話，建議改放 `CLAUDE.md`（通則）或 PR description（單純 bug），不要硬寫進
`LESSONS.md`。

---

### Step 2 — 從對話 context 草擬四個欄位

根據剛剛發生的事（使用者的糾正、bug 追查過程、review 發現），草擬：

- **症狀指紋**：下次怎麼一眼認出來——用具體、可驗證的特徵（錯誤訊息片段、行為描述、觸發條件），不要寫「注意這個地方容易錯」這種模糊描述
- **根因**：為什麼會發生——是誤解了什麼架構假設
- **修法**：實際改了什麼、在哪個檔案（`path/to/file.ts` 或 `file:line`）
- **以後的追問**：遇到類似情境，下次要先問自己或使用者什麼問題，才能提早攔下來

---

### Step 3 — 給使用者確認

把草擬的四個欄位直接列出來給使用者看，讓他確認或修改。**不用多輪 Q&A**，這個 skill
的重點是快——草稿不對就直接請他改字句，不用重新走一輪引導式提問。

---

### Step 4 — 決定標題與日期

- 日期：今天（`YYYY-MM-DD`）
- 標題：簡短、人類可讀，描述症狀本身（例：「Preview deploy 誤打 production API」），不是解法

---

### Step 5 — 寫入 LESSONS.md

在 `LESSONS.md` 的「條目」這節底下（`（尚無正式條目）` 若還在，先刪掉），依日期由舊到新新增一條：

```markdown
### YYYY-MM-DD — [標題]

- **症狀指紋**：[內容]
- **根因**：[內容]
- **修法**：[內容]
- **以後的追問**：[內容]
```

---

### Step 6 — Commit

```bash
git add LESSONS.md
git commit -m "docs(lessons): [標題]"
```

若目前在 feature branch 上，且這個教訓跟當前 branch 的改動直接相關，可以併入當前的 commit 而不是獨立
commit——依情況判斷，不確定就問使用者。

---

## 注意事項

- 四個欄位都要填，症狀指紋是最容易寫得太模糊的一項——寫完檢查一次：如果只看這一行，下次遇到同樣症狀能不
  能認出來？認不出來就重寫
- 這個 skill 只動 `LESSONS.md`，不動其他文件
- 立刻寫，不要等到收尾或下班前才補——記憶會流失細節，尤其是「一開始的錯誤假設是什麼」這件事
