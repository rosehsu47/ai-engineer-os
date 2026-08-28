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
