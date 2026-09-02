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

想讓沒在跑的 repo 卡片上出現「啟動 supervisor」表單（見下），再多帶一個 flag：

```bash
aios-panel -repos /path/a,/path/b \
  -supervisor-script /path/to/ai-engineer-os/supervisor/supervisor.sh
```

想讓有 dev_command 的 repo 卡片上出現「📌 開機自動啟動」開關（見下「dev
server 開機自動啟動」），再多帶一個 flag：

```bash
aios-panel -repos /path/a,/path/b \
  -devserver-launchd-script /path/to/ai-engineer-os/panel/devserver-launchd-install.sh
```

三個 `-*-script` flag 可以一起帶。

## 開機/登入自動啟動

手動 `go run .`／`aios-panel` 起的 process，重開機或當掉就沒了。想要
panel 真的常駐（跟 kotoba 那種靠 `supervisor/schedule-install.sh` 裝的
定時 supervisor job 不一樣——那是「固定時刻跑一輪」，這裡是「開機/登入
就啟動、當掉自動重開」）：

```bash
panel/launchd-install.sh              # 安裝/更新（讀 ~/bin/aios-panel，帶
                                       # -dashboard-script/-supervisor-script/
                                       # -devserver-launchd-script）
panel/launchd-install.sh --status     # 看目前載入狀態
panel/launchd-install.sh --uninstall  # 移除（不會砍掉正在跑的 process）
panel/launchd-install.sh --dry-run    # 只印 plist 不動系統
```

用 macOS 原生 launchd（`RunAtLoad` + `KeepAlive`），單例（整台機器一份，
不分 repo，label 固定 `com.aios.panel`）。要先 `go build -o ~/bin/aios-panel .`
把 binary 生出來，腳本會檢查、沒有就提示怎麼 build。log 在
`~/.aios-panel-state/launchd.log`。repo 清單一樣讀 `~/.aios-repos`，
熱重載，不用重裝 job 就能加新 repo。

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

### dev server 開機自動啟動（選用，逐 repo 裝）

想要某個 repo 的 dev server 常駐（開機/登入就自動起來，不用每次自己按
「▶ 啟動」）——跟 panel 本體的常駐（見上）是同一套 launchd 機制，但**只用
`RunAtLoad`、不用 `KeepAlive`**：你在 panel 卡片按「■ 停止」殺掉之後就是
真的停了，不會被搶救回來，下次登入才會再自動啟動。這是刻意的：dev server
會被手動停掉是常態（切換分支、重跑一次乾淨的），`KeepAlive` 會讓停止鍵
形同虛設。

```bash
panel/devserver-launchd-install.sh --repo /path/to/repo              # 安裝
panel/devserver-launchd-install.sh --repo /path/to/repo --status     # 看狀態
panel/devserver-launchd-install.sh --repo /path/to/repo --uninstall  # 移除
panel/devserver-launchd-install.sh --repo /path/to/repo --dry-run    # 只印 plist
```

指令讀 `~/.aios-repos` 該 repo 那一行的第三欄起（沒設定會直接報錯提示先
加上）；pid／log 檔跟 panel 自己啟動 dev server 時寫的是同一份
（`~/.aios-panel-state/{slug}.pid`／`.log`），所以不論這個 process 是開機
時 launchd 起的、還是你在 panel 卡片按的，panel 卡片的狀態燈、log 連結、
■ 停止鍵完全認得、不用改任何程式碼。label 固定 `com.aios.devserver.{slug}`
（`{slug}` 跟 panel 內部用的是同一個 sanitize 規則），逐 repo 各裝一份，
彼此獨立。

**也可以直接在 panel 卡片上按**（不用開終端機）：有帶 `-devserver-launchd-script`
時，dev server 那一列會多一顆「📌 設開機自動啟動」／「📌 開機自動啟動」
開關，按一下就是呼叫這支腳本裝/卸（`repo` 走 argv 傳給 `exec.Command`，
不經過 shell 展開，沒有指令注入面，跟啟動 supervisor 那個 endpoint 同一
信任層級）。卡片上顯示的「已裝」狀態純看 plist 檔存不存在，跟這個 repo
的 dev server 現在是不是正在跑無關——裝了之後不會立刻啟動，要等下次
登入，或你自己按「▶ 啟動」。

## 畫面上有什麼（每 5 秒自動更新）

標題列下方會顯示本機內網 IP（純參考用——panel 仍只綁 127.0.0.1，這個
位址目前連不進來；要從手機/其他裝置連，需要額外的 tunnel 並自行評估
沒有認證這件事的風險）。

卡片依狀態分成 6 組顯示（依需要處理的急迫度排序，組與組之間有標題列）：
❓ 需要你回覆 → 🔴 已煞車 → 🟢 執行中 → 🔵 已回覆待下一輪 → ⚪ 待命 →
⚫ 無待辦（尚未 `/ai-init` 的 repo 額外獨立一組排最後）。⚪ 待命跟
⚫ 無待辦的差別只在 backlog 是否為空——⚫ 沒有排隊中的任務，就算按了
「啟動 supervisor」也只會立刻因為 `QUEUE_EMPTY` 收工，用不同分類提醒
「這張卡不用你操心，但也沒什麼好啟動的」。

**鍵盤導覽**：`j`/`k` 或 ↑/↓ 跨分組切換卡片焦點（藍色外框標示目前選中的
卡）；`Enter`——該卡有待回覆問題就把游標直接送進回覆欄位，沒有就捲動
置頂該卡；游標在回覆欄位裡時 `Cmd/Ctrl+Enter` 直接送出回覆。焦點靠
repo path 記憶，5 秒重繪、卡片因狀態變動換組都不會跑掉。

每個 repo 一張卡：
- 狀態燈：🟢 執行中（含 pid）／⚪ 待命／⚫ 無待辦／❓ 等你回答／
  🔵 已回覆待下一輪／🔴 已煞車。🟢 執行中**不分是 supervisor.sh 還是
  互動 session**——兩種鎖（`.ai/supervisor/lock`、`.ai/state/session.lock`，
  見 `AI-RUNTIME.md` 單一寫入者不變量）任一個活著就算執行中，卡片內文
  會註明是哪一種（supervisor 給 pid+log 連結；互動 session 只給 pid，
  沒有 log 檔可看）。**`session.lock` 是 `/ai-work`／`/ai-review` 自己
  寫的**，只有跑過 `/ai-sync` 補到最新 skill 版本的 repo 才會有這個訊
  號——舊 repo 沒同步之前，互動 session 執行中不會反映在這裡，只有
  supervisor.sh 那條腿看得到
- checkpoint phase 與輪數、上輪結果與成本
- **啟動 supervisor**（有帶 `-supervisor-script`、目前沒在跑、也沒被 STOP
  時才出現；有互動 session 在跑時也不出現，現在啟動只會立刻撞
  session lock 收工）：等同 `supervisor/supervisor.sh --repo <repo> ...`，
  表單開放 `--model`（opus/sonnet/haiku 下拉）、`--quota-wait`、
  `--quota-stop`、`--max-iterations`、`--max-failures`、`--claude-flags`
  （原始文字，附加給 claude CLI），以及
  `--once`／`--review`／`--wait-on-pause`／`--ignore-quota`／`--yolo`
  五個開關（勾 `--yolo` 會多一次瀏覽器 confirm）；留白的欄位吃
  `.ai/schedule.yml` 的預設值，跟直接下指令一樣。執行中會顯示 pid 與
  log 連結（純文字，開新分頁看 stdout/stderr），同一列還有兩顆停止類
  按鈕，特意用顏色分級拉開差距（不然文字都帶「停」、顏色又都紅，光看
  不看 tooltip 分不出差別——實測踩過這個坑）：**STOP**（中性灰，寫
  `.ai/STOP`，跟卡片下方那顆大的 STOP 是同一個信號旗機制，這裡是就近
  的捷徑；supervisor.sh 自己的迴圈下個檢查點會偵測到、優雅退出，不會
  打斷正在跑的這一輪，是「正常、隨時可按」的動作）；**目前偵測到閒置
  中**（沒有 `/ai-work` 或 `/ai-review` 正在跑——即 `.ai/state/session.lock`
  沒被持有，代表 supervisor.sh 正卡在 iteration 之間的 sleep／quota-wait
  輪詢，不是在跑任務）時，多一顆紅色的**FORCE STOP**：直接
  `SIGTERM`/`SIGKILL` 那個 process group，不等它跑到下個檢查點，這一輪
  不能 resume——紅色留給它，因為它才是真的不可逆、少用的那個；正在跑
  任務時這顆按鈕不會出現，只顯示原因，逼你用 STOP 或等它閒置
- 進行中任務、待辦前 5 筆＋總數、完成數、最近 3 張收據——待辦超過 5 筆
  時多一顆「顯示全部 N 筆待辦」按鈕，按需向 `/api/backlog` 抓完整清單
  （不進 5 秒常態輪詢，避免拖慢畫面），收合按鈕縮回去；展開狀態撐得過
  5 秒自動重繪。**排序照 `priority`（同分取 `created_at` 最舊）**，跟
  `/ai-work` 步驟 2 選任務的規則一致——不是 `backlog.yaml` 檔案裡的原始
  寫入順序，這樣看到排第一筆的就真的是下一輪最可能被挑中的那筆
- **dev server 啟動/停止**（第三欄設了指令才會出現）：▶ 啟動／■ 停止
  按鈕＋執行中/未啟動狀態，執行中會顯示 pid 與一個 log 連結（純文字，
  開新分頁看 `sh -c "{指令}"` 的 stdout/stderr）；有帶
  `-devserver-launchd-script` 時同一列多一顆「📌 開機自動啟動」開關
  （見上「dev server 開機自動啟動」），裝了之後開機/登入會自動啟動這個
  dev server，用旁邊的 ■ 停止關掉就是真的關了（沒有 KeepAlive，不會被
  拉回來）
- **❓ 問答區**：agent 的 PAUSED 問題直接顯示，textarea 送出回覆
- **🚢 出貨提示**：ai/queue 領先幾個 commit＋可複製的 `/ai-ship` 指令
- **STOP 煞車／解除**按鈕
- **📊 儀表板**圖示按鈕（卡片標題列右側，`.ai/reports/dashboard.html`
  存在、或有帶 `-dashboard-script` 時才出現）：開新分頁看 `dashboard.sh`
  渲染的任務統計/收據表/git 事件。有帶 `-dashboard-script` 時，點開若
  快照超過 1 分鐘會先重算；沒帶就只讀既有檔案，不會自動更新
- **⚙ 排程設定**圖示按鈕（`.ai/schedule.yml` 存在時才出現）：開新分頁到
  `/schedule?repo=...`——一個可編輯的表單頁（跟 `/ai-config` skill 問的
  是同一組 16 個 key，分五類分組），改完按「儲存」直接寫回
  `.ai/schedule.yml`，改完立即生效、不用重啟任何東西。表單值由
  `/api/schedule-config`（GET 讀現況、POST 存檔）提供，跟主頁 SPA 同一套
  fetch 模式；只精準取代有改動的那幾行，其餘行與註解不動（跟 `/ai-config`
  skill 步驟 4 同一套紀律，見 `panel/schedule.go`）。頁面上也留了「查看
  原始檔案」連結（`/api/schedule` 直接 `ServeFile`，純唯讀），想單純確認
  目前值、不想手滑改到的話可以用這個。**panel 只存檔，不會自動 git
  commit**——跟 `/ai-config` skill 會順手 commit 不一樣，改完記得自己
  進 repo `git add .ai/schedule.yml && git commit`

## 設計原則（為什麼它做不了更多）

panel 只是**協定檔的讀者與寫者**——判斷力留在 agent：
- 回覆只是「附寫進 `.ai/PAUSED` 的 `## 人類回覆` 節」；怎麼路由到任務/
  記憶由下一輪 `/ai-work` 統一處理（跟 `/ai-answer` 寫的回覆走同一條路）
- STOP/恢復 = 建立/刪除信號旗檔案
- **排程設定表單寫 `.ai/schedule.yml`**：這份本來就是純數字/開關的人類
  調參檔（agent 也不能碰，deny 名單上），不是判斷力/協定執行檔，寫入
  不用經過任何 agent 決策，性質跟前兩者一樣安全
- **出貨（git push）與 merge 永遠不在 panel 裡發生**——那是對外動作，
  留在你的終端機與 GitHub

**有三個例外真的會在本機 spawn 長駐/一次性行程或動系統層設定，不是協定
檔讀寫**——刻意跟協定狀態分開處理，把風險收斂到最小：

**dev server 啟動/停止**：
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

**啟動 supervisor**（需 `-supervisor-script`，見上）：
- 表單值全部走 `exec.Command` 的 argv、不經過 shell 字串展開，跟
  dev server「使用者自維護指令字串」性質不同，本來就沒有指令注入面；
  `--model` 額外限制只能是 opus/sonnet/haiku 白名單，其餘數字欄位驗證
  是合法整數才放行
- 啟動前用 `.ai/supervisor/lock` 檔擋「已經在跑」，跟 supervisor.sh
  自己的單 repo 單 supervisor 鎖是同一份，不會重複啟動
- panel **不**額外維護一份自己的 pid/存活狀態——`.ai/supervisor/lock`
  是 supervisor.sh 自己寫入、EXIT trap 保證清掉的協定檔，卡片上的
  🟢/⚪ 狀態本來就讀這份，啟動表單只是幫你把指令組好、按下去
- 優雅停止用卡片下方既有的 STOP 按鈕（寫 `.ai/STOP`），跟你自己在終端機
  `touch .ai/STOP` 效果相同，supervisor.sh 的迴圈本來就會偵測。log 只到
  `~/.aios-panel-state/`，不影響 `.ai/` 底下 supervisor.sh 自己寫的
  events/receipts 稽核紀錄
- **FORCE STOP**（`/api/supervisor-kill`）：STOP 要等 supervisor.sh 自己在
  安全點退出（可能還在跑一輪 `/ai-work`），有時候等不了那麼久。這個
  endpoint 直接 `SIGTERM`（逾時 `SIGKILL`）整個 process group，但**只在
  確認閒置時才放行**——伺服器端強制檢查 `.ai/state/session.lock` 沒被
  持有（`.ai/supervisor/lock` 活著時，session lock 只可能是 supervisor
  自己這輪 `/ai-work`／`/ai-review` 持有的，單一寫入者不變量保證），
  代表 supervisor.sh 正卡在 iteration 之間的 `sleep`／quota-wait 輪詢，
  沒有任何檔案正在被寫，直接殺掉不會留下半寫入的狀態；正在跑任務時
  這個 endpoint 會擋下來（409，附原因），前端也不會顯示按鈕。這是刻意
  不對稱的設計：**只做安全時才允許的 FORCE STOP，不做任意時刻的強制
  kill**——後者需要更複雜的中斷點設計（例如任務中途的部分回滾），
  現在故意不做

**dev server 開機自動啟動**（需 `-devserver-launchd-script`，見上）：
- `repo` 值走 `exec.Command` 的 argv 傳給
  `panel/devserver-launchd-install.sh`，不經過 shell 展開，沒有指令注入
  面；實際指令內容照舊只能來自 `~/.aios-repos`，跟 dev server 啟動/停止
  同一個信任層級
- 唯一會動系統層設定的功能——會寫 `~/Library/LaunchAgents/*.plist`、呼叫
  `launchctl bootstrap/bootout`，這兩個是 macOS 帳號層級的設定，不是
  panel 自己管得到的檔案；panel 只負責照按鈕動作呼叫腳本，plist 內容/
  launchctl 呼叫全部在腳本裡，panel 沒有自己組 XML
- 卡片顯示的「已裝」狀態純看 plist 檔存不存在（`os.Stat`），不會另外呼叫
  `launchctl list` 查即時狀態——這是唯讀判斷，跟腳本自己的 `--status`
  分開，不會互相影響

所以 panel 壞了/沒開，系統照常運作；它沒有任何獨占的協定狀態（這三個
才是會留下本機執行副作用的功能，都跟 `.ai/` 協定本身的正確性無關）。
