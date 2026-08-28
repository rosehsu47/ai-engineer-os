# Lessons Learned

這份檔案給未來的 session（人類或 AI）讀。

**什麼時候寫**：使用者糾正了一個錯誤假設、bug 是因為誤解架構造成的、或 review
抓到一個「早知道就不會犯」的問題——立刻寫，不要等收尾。

**每條的格式**：
- **症狀指紋** — 下次怎麼一眼認出來（可驗證的特徵，不是模糊描述）
- **根因** — 為什麼會發生
- **修法** — 改了什麼、在哪個檔案
- **以後的追問** — 遇到類似情境要先問什麼問題

**不要寫**：一般性的最佳實踐（那些放 `CLAUDE.md`）、單純的 bug 修復紀錄（那些放 receipt / PR
description）。只寫「這個專案特有的、直覺會出錯的地方」。

---

## 格式範例

> 以下是範例，不是正式條目，示範怎麼填四個欄位。新條目寫在「條目」這節下面，依日期由舊到新排列。

### 2026-01-01 — 範例：Cloudflare Pages preview 誤判成 production

- **症狀指紋**：PR preview deploy 失敗，錯誤訊息包含 `Unexpected non-whitespace character after JSON`，但本地 build 完全正常
- **根因**：誤以為 preview 環境會打 preview API，實際上 preview build 打的是 production API endpoint，而該 endpoint 當時還沒部署
- **修法**：改 `web/src/lib/api-client.ts` 讓 build-time fetch 依環境變數切換 API base URL
- **以後的追問**：遇到「preview 壞掉但本地正常」時，先確認 preview build 實際打的是哪個 API 環境，不要假設跟本地一致

---

## 條目

### 2026-08-28 — macOS bash 3.2 下 $var 緊接全形字元被吃進變數名

- **症狀指紋**：bash 腳本印出 `<變數名><緊貼的中文/全形標點>: unbound variable`（例如 `OUT：unbound variable`），`set -u` 下直接把腳本整個殺掉，錯誤行號常常指向看起來完全無關的行
- **根因**：macOS 系統內建 bash 是 3.2（2007 年的版本），解析 `$var` 後面緊鄰的多位元組字元時會把它併入變數名去查找，而不是當成變數名結束的邊界字元
- **修法**：`scripts/gen-changelog.sh` 的 `echo "已重新產生 $OUT（掃了..."` 改成 `${OUT}（...`。一般解法：任何 zh-Hant 訊息字串裡，`$var` 後面緊接非 ASCII 字元（全形括號、標點）一律要寫 `${var}`
- **以後的追問**：寫/改這個 repo 裡任何 bash 腳本前，先跑一次 `grep -nE '\$[A-Za-z_][A-Za-z0-9_]*[^\x00-\x7F]' <file>` 掃有沒有裸變數貼著全形字元；新增訊息字串時直接養成 `${var}` 的習慣，不要等出事才補

### 2026-08-29 — session.lock 用純 kill -0 判斷存活，撞到 pid 回收誤判成孤兒鎖卡死

- **症狀指紋**：panel 卡片持續顯示「🧑‍💻 有人正在互動執行（pid N）」數小時不消失，但該 repo 實際上沒有任何 `/ai-work`／`/ai-review` 在跑；`cat .ai/state/session.lock` 給出的 pid 用 `ps -p <pid>` 一查，是一個啟動時間遠早於 lock 檔 mtime、command 完全無關的行程
- **根因**：`.ai/state/session.lock` 的存活判斷原本只看 `kill -0 <pid>`——單次 `/ai-work` 呼叫被 watchdog 砍掉時沒機會清空自己的 lock，殘留的 pid 之後可能被作業系統回收給不相干的行程，`kill -0` 對這種「巧合存活」一樣回傳成功，孤兒 lock 就永遠不會被判定過期
- **修法**：`AI-RUNTIME.md` 補上「session.lock 存活要同時滿足 `kill -0` 成功且檔案 mtime 在 2 小時內」的規則；`templates/skills/ai-work`、`ai-review`、`ai-task`、`ai-wrap` 的 lock 檢查步驟同步加上 mtime 門檻；`panel/main.go` 的 `sessionLockStatus()` 加 `sessionLockMaxAge`（2 小時）常數與 `time.Since(info.ModTime())` 檢查。**只套用在 session.lock**，`supervisor/lock` 橫跨整個無人迴圈，合法存活時間本來就可能數小時以上，不能套同一個門檻
- **以後的追問**：任何用 pid 檔案判斷「行程還在跑」的機制，都要問「如果這個 pid 已經死了、被系統回收給別的行程，我的檢查會不會被騙？」——純 `kill -0` 永遠不夠，要嘛疊時間戳門檻，要嘛記錄行程啟動時間一起比對
