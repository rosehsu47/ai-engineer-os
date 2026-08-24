# aios-panel — 本機控制台

一頁看所有 repo 的 agent 狀態，就地回答問題、踩煞車。**零外部依賴**
（Go 標準庫），只綁 127.0.0.1、無認證——僅供本機使用。

```bash
cd ai-engineer-os/panel
go build -o ~/bin/aios-panel .        # 或 go run .
aios-panel -repos /path/a,/path/b     # 或把路徑一行一個寫進 ~/.aios-repos
open http://127.0.0.1:7777
```

想讓卡片上的「📊 儀表板」連結能自動重算（見下），多帶一個 flag：

```bash
aios-panel -repos /path/a,/path/b \
  -dashboard-script /path/to/ai-engineer-os/supervisor/dashboard.sh
```

repo 清單**熱重載**：`~/.aios-repos` 每次輪詢重讀，新 repo append 進去
（/ai-init 收尾會自動做）5 秒內卡片就出現，panel 不用重啟。

每行可選填第二欄（空白分隔）：這個 repo 本機 dev server 的網址；選填
第三欄起：啟動 dev server 的指令（有填才會出現「▶ 啟動」按鈕），例如：

```
/path/a http://localhost:5173 npm run dev
/path/b http://localhost:8080/ make dev
```

有填網址的話卡片標題列會多一個 ↗ 圖示，點開新分頁直接開那個網址。純
人工維護的清單欄位，不是 `.ai/` 協定檔，agent 不會讀寫它；`/ai-init`
訪談時會問一次，之後想改就直接編輯 `~/.aios-repos`。

## 畫面上有什麼（每 5 秒自動更新）

標題列下方會顯示本機內網 IP（純參考用——panel 仍只綁 127.0.0.1，這個
位址目前連不進來；要從手機/其他裝置連，需要額外的 tunnel 並自行評估
沒有認證這件事的風險）。

卡片依狀態分成 5 組顯示（依需要處理的急迫度排序，組與組之間有標題列）：
❓ 需要你回覆 → 🔴 已煞車 → 🟢 執行中 → 🔵 已回覆待下一輪 → ⚪ 待命
（尚未 `/ai-init` 的 repo 額外獨立一組排最後）。

**鍵盤導覽**：`j`/`k` 或 ↑/↓ 跨分組切換卡片焦點（藍色外框標示目前選中的
卡）；`Enter`——該卡有待回覆問題就把游標直接送進回覆欄位，沒有就捲動
置頂該卡；游標在回覆欄位裡時 `Cmd/Ctrl+Enter` 直接送出回覆。焦點靠
repo path 記憶，5 秒重繪、卡片因狀態變動換組都不會跑掉。

每個 repo 一張卡：
- 狀態燈：🟢 執行中（含 pid）／⚪ 待命／❓ 等你回答／🔵 已回覆待下一輪／🔴 已煞車
- checkpoint phase 與輪數、上輪結果與成本
- 進行中任務、待辦前 5 筆＋總數、完成數、最近 3 張收據
- **dev server 啟動/停止**（第三欄設了指令才會出現）：▶ 啟動／■ 停止
  按鈕＋執行中/未啟動狀態，執行中會顯示 pid 與一個 log 連結（純文字，
  開新分頁看 `sh -c "{指令}"` 的 stdout/stderr）
- **❓ 問答區**：agent 的 PAUSED 問題直接顯示，textarea 送出回覆
- **🚢 出貨提示**：ai/queue 領先幾個 commit＋可複製的 `/ai-ship` 指令
- **STOP 煞車／解除**按鈕
- **📊 儀表板**圖示按鈕（卡片標題列右側，`.ai/reports/dashboard.html`
  存在、或有帶 `-dashboard-script` 時才出現）：開新分頁看 `dashboard.sh`
  渲染的任務統計/收據表/git 事件。有帶 `-dashboard-script` 時，點開若
  快照超過 1 分鐘會先重算；沒帶就只讀既有檔案，不會自動更新
- **🕐 排程設定**圖示按鈕（`.ai/schedule.yml` 存在時才出現）：開新分頁
  看該 repo `schedule.yml` 的原始內容（純唯讀，`/api/schedule` 直接
  `ServeFile`）——想確認 `schedule_start_times`、`max_cost_per_run_usd`
  這類參數目前設什麼值，不用自己開終端機 cat 檔案

## 設計原則（為什麼它做不了更多）

panel 只是**協定檔的讀者與寫者**——判斷力留在 agent：
- 回覆只是「附寫進 `.ai/PAUSED` 的 `## 人類回覆` 節」；怎麼路由到任務/
  記憶由下一輪 `/ai-work` 統一處理（跟 `/ai-answer` 寫的回覆走同一條路）
- STOP/恢復 = 建立/刪除信號旗檔案
- **出貨（git push）與 merge 永遠不在 panel 裡發生**——那是對外動作，
  留在你的終端機與 GitHub

**dev server 啟動/停止是唯一的例外**——它不是協定檔讀寫，而是真的會在
本機 spawn 一個長駐行程。刻意跟協定狀態分開處理，把風險收斂到最小：
- 指令只能來自你自己維護的 `~/.aios-repos`（本機檔案，不是網路輸入、
  agent 也不會寫它），不存在指令注入的外部攻擊面
- pid/log 檔存在 `~/.aios-panel-state/`，完全不碰目標 repo 的 `.ai/`——
  跟審計/協定狀態切乾淨，panel 壞掉也不會弄髒 repo 的稽核紀錄
- 啟動時用獨立 process group（`Setpgid`），停止時整個 group 一起收，
  `npm run dev` 這類會 fork 子行程（`next-server`）的指令不會留孤兒行程
- **啟動前一定先檢查設定的 port 有沒有人在聽**，不只看 panel 自己的
  pidfile——如果你自己在終端機手動開過（或指令沒寫死 `--port`，服務
  本身會在偵測到 port 被占用時自己悄悄換一個），panel 會辨識成「已經
  在跑」而不重複啟動，避免多開一份、換到意料外的 port。這種情況下
  panel 沒有可控制的 pid，停止鍵會如實回報「不是我啟動的，你自己關」，
  不會假裝關掉了

所以 panel 壞了/沒開，系統照常運作；它沒有任何獨占的協定狀態（dev
server 是唯一會留下本機執行副作用的功能，跟 `.ai/` 協定本身無關）。
