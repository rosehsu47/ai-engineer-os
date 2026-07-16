# aios-panel — 本機控制台

一頁看所有 repo 的 agent 狀態，就地回答問題、踩煞車。**零外部依賴**
（Go 標準庫），只綁 127.0.0.1、無認證——僅供本機使用。

```bash
cd ai-engineer-os/panel
go build -o ~/bin/aios-panel .        # 或 go run .
aios-panel -repos /path/a,/path/b     # 或把路徑一行一個寫進 ~/.aios-repos
open http://127.0.0.1:7777
```

repo 清單**熱重載**：`~/.aios-repos` 每次輪詢重讀，新 repo append 進去
（/ai-init 收尾會自動做）5 秒內卡片就出現，panel 不用重啟。

## 畫面上有什麼（每 5 秒自動更新）

每個 repo 一張卡：
- 狀態燈：🟢 supervisor 執行中（含 pid）／⚪ 待命／🟡 等你回答／🔴 已煞車
- checkpoint phase 與輪數、上輪結果與成本
- 進行中任務、待辦前 5 筆＋總數、完成數、最近 3 張收據
- **❓ 問答區**：agent 的 PAUSED 問題直接顯示，textarea 送出回覆
- **🚢 出貨提示**：ai/queue 領先幾個 commit＋可複製的 `/ai-ship` 指令
- **STOP 煞車／解除**按鈕

## 設計原則（為什麼它做不了更多）

panel 只是**協定檔的讀者與寫者**——判斷力留在 agent：
- 回覆只是「附寫進 `.ai/PAUSED` 的 `## 人類回覆` 節」；怎麼路由到任務/
  記憶由下一輪 `/work` 統一處理（跟 `/ai-answer` 寫的回覆走同一條路）
- STOP/恢復 = 建立/刪除信號旗檔案
- **出貨（git push）與 merge 永遠不在 panel 裡發生**——那是對外動作，
  留在你的終端機與 GitHub

所以 panel 壞了/沒開，系統照常運作；它沒有任何獨占的狀態。
