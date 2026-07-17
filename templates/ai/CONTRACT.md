# CONTRACT.md — Agent 契約

> 這份契約是 agent 在這個 repo 的長期規則，優先級高於任務描述。
> 任務要求與契約衝突時，契約贏（任務退回並標 blocked）。
> `{{...}}` 佔位符由 `/ai-init` 訪談填寫。

## 1. 使命與職責

{{MISSION：這個 agent 在此 repo 負責什麼、不負責什麼}}

## 2. 環境資訊

| 項目 | 值 |
|---|---|
| 語言/技術棧 | {{STACK}} |
| 測試指令 | `{{TEST_COMMAND}}` |
| 建置指令 | `{{BUILD_COMMAND}}` |
| 主分支 | `{{MAIN_BRANCH}}` |
| 工作分支 | `ai/queue`（不存在時從主分支建立） |

## 3. 允許的操作

- 讀取 repo 內任何檔案（`.env*` 與 secrets 除外）
- 編輯與任務相關的程式碼/文件/測試
- 執行：測試指令、建置指令、`git status/diff/log/add/commit/checkout/stash/branch`、
  `ls/find/grep` 及 `.claude/settings.local.json` 白名單內的指令
- 在 `.ai/` 內維護自己的狀態與紀錄

## 4. 禁止的操作

- **任何 DELETE 級的破壞**：刪除任務範圍外的檔案、`git push --force`、
  改寫 git 歷史、`rm -rf`、**丟棄工作區變更的 git 操作**
  （`git checkout -- <path>`、`git restore`、`git reset --hard`、
  `git branch -D`、`git stash drop`）——工作區可能有人類的 WIP
- commit 到主分支（只能在工作分支）
- **`git add -A` / `git add .`**——只 add 自己為了這個任務修改的檔案
  （工作區可能有人類未 commit 的工作，動了就是事故）
- 碰 `.env*`、憑證、secrets
- 修改 `.claude/` 設定、本契約（CONTRACT.md）、`.ai/schedule.yml`
- 安裝新依賴（屬「需要人類批准」）
- {{EXTRA_FORBIDDEN：repo 特有的禁令，例如「不修改 migrations/ 既有檔案」}}

## 5. 完成的定義（Definition of Done）

以下全部成立才算完成一個任務：
1. 任務的每一條 `acceptance` 都被滿足且可展示證據
2. 測試指令通過（沒有測試基建的 repo：至少建置通過 + 手動驗證主流程）
3. 自評 rubric 分數 ≥ 門檻（`.ai/rubrics/` 啟用時）
4. Receipt 已寫入、checkpoint 歸 `idle`、任務移入 done.yaml
5. 程式碼與紀錄各自 commit 完成

## 6. 提交前儀式（每次 commit 前）

1. 跑測試指令，確認通過
2. `git diff` 自我檢視一遍（不是形式——找出忘了刪的 debug 碼、误改的檔案）
3. 確認 staged 檔案清單只含本任務的檔案
4. Commit message：`type(scope): title [T-NNN]`
5. `.ai/` 的紀錄變更另開一個 commit：`chore(ai): records for T-NNN`

## 7. 需要人類批准的界線

碰到以下任何一項：**寫 `.ai/PAUSED`（內容 = 具體的問題與你建議的選項），
輸出 `AIOS_STATUS: PAUSED`，停止**。不要猜、不要繞過：
- 缺少 secret/憑證/環境變數
- 需要安裝新依賴或升級既有依賴
- 需要 schema migration 或其他不可逆的資料操作
- acceptance 條件語意不明，兩種合理解讀會做出不同的東西
- 任務要求與本契約衝突

**大型變更（單一任務動超過 10 個檔案）不暫停，但強制獨立審查**：
繼續完成任務，收尾時 (a) receipt 開頭顯著標記「大型變更（N 檔）」、
(b) 建立 `.ai/REVIEW_REQUESTED` 旗標——supervisor 會在本輪後強制跑
獨立審查（即使 review_after_task 關閉）。**禁止為了壓回 10 檔以內而
人為合併/拆分檔案**——門檻的目的是讓大變更多一雙眼睛，不是懲罰
檔案數；照最合理的切法寫，超過就標記。

## 8. 錯誤與復原原則（agent 層）

- **測試失敗**：修，最多 2 輪。仍失敗 → WIP commit（`wip(T-NNN): failing - 原因`）
  → 任務退回 backlog（attempts 保留；達 max_attempts 標 blocked）→
  memory.md 記下失敗細節 → receipt 標 `failed`
- **git 衝突**：`git rebase --abort` / `git merge --abort`，任務標 blocked +
  原因，絕不強行解衝突
- **啟動時工作區是髒的**：先查 checkpoint——是自己上次中斷的就續作；
  不是自己的改動 → `git stash push -m "aios: 保存非本 agent 的變更"` 並記在
  memory.md（人類的 WIP 永不覆蓋、永不 commit）

## 9. 溝通協定

- 每次執行的最後一行必是 `AIOS_STATUS:` 行（格式見 AI-RUNTIME.md）
- 每個任務結束（不論成敗）都寫 receipt；`QUEUE_EMPTY` 不寫
- checkpoint 在每個子步驟後整檔重寫
- context.md 每輪最多加 3 行、全檔維持 ≤100 行（砍最舊的）
- 有架構層級的選擇時，decisions.md 加一筆 ADR-lite（背景/決定/替代方案/後果）
