package main

// pageHTML：單頁控制台。每 5 秒輪詢 /api/state 重繪；
// 協定檔動作是兩種 POST（回答 PAUSED、STOP/恢復），出貨顯示指令供複製；
// 另外兩種 POST 是真的 spawn 行程（dev server 啟停、啟動 supervisor），
// 見 main.go 開頭的設計原則註解。
const pageHTML = `<!DOCTYPE html><html lang="zh-Hant"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AI Engineer OS — 控制台</title>
<style>
 body{font-family:-apple-system,'PingFang TC',sans-serif;background:#0a0f1c;color:#e2e8f0;margin:0 auto;padding:20px 28px;max-width:1100px}
 h1{font-size:18px} .muted{color:#8b98ac;font-size:12px}
 .grid{display:flex;flex-direction:column;gap:8px;margin-top:14px}
 .repo{background:#182238;border:1px solid #2e3c5e;border-radius:14px}
 .repo.focused{border-color:#38bdf8;box-shadow:0 0 0 2px #38bdf855}
 .rrow{display:flex;align-items:center;gap:10px;padding:12px 18px;cursor:pointer}
 .rrow:hover{background:#212c4a}
 .rrow .rname{font-weight:700;color:#f1f5f9;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;flex-shrink:0}
 .rrow .rmeta{color:#94a3b8;font-size:12.5px;flex:1;min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
 .rrow .rstats{color:#8b98ac;font-size:11.5px;flex-shrink:0;font-variant-numeric:tabular-nums}
 .rrow .rtime{color:#8b98ac;font-size:11.5px;flex-shrink:0;font-variant-numeric:tabular-nums;width:2.5em;text-align:right}
 .rrow .chevron{color:#64748b;flex-shrink:0;transition:transform .15s}
 .repo.expanded .chevron{transform:rotate(90deg)}
 .rbody{padding:6px 22px 22px;border-top:1px solid #2a3654;display:flex;flex-direction:column}
 .dot{width:8px;height:8px;border-radius:99px;display:inline-block;flex-shrink:0}
 .running{background:#34d399}.idle{background:#64748b}.stopped{background:#ef4444}.paused{background:#f59e0b}.waiting{background:#38bdf8}.missing{background:#334155}
 .status-header{font-size:13px;font-weight:700;color:#e2e8f0;margin:18px 0 6px;padding-bottom:6px;border-bottom:1px solid #263047;display:flex;align-items:center;gap:8px}
 .status-header:first-child{margin-top:0}
 .status-header .count{font-weight:400;color:#8b98ac;font-size:12px}
 .row{font-size:12.5px;margin:12px 0 4px;color:#94a3b8}
 .stats{display:flex;align-items:center;gap:12px;font-size:12.5px;color:#94a3b8;margin:14px 0}
 .stats .bar{flex:1;height:6px;background:#1e293b;border-radius:99px;overflow:hidden}
 .stats .bar i{display:block;height:100%;background:#34d399;border-radius:99px}
 .stats .pct{color:#cbd5e1;font-variant-numeric:tabular-nums;font-weight:700}
 .section-label{font-size:14px;font-weight:700;color:#f1f5f9;margin:20px 0 10px}
 /* 圓角只留右邊——border-left 當強調色條配上四角都圓，左邊那條會被
    切成尖角/括號狀（border 在圓角處是斜接的），只圓右邊角才會是乾淨
    的矩形色條 */
 .task-card{display:flex;gap:12px;align-items:flex-start;background:#0d1526;border-left:4px solid #34d399;border-radius:0 12px 12px 0;padding:12px 14px;margin:6px 0}
 .task-row{display:flex;gap:12px;align-items:flex-start;border-left:3px solid #38bdf866;padding:8px 12px;margin:6px 0}
 .task-id{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;white-space:nowrap;padding-top:2px}
 .task-card .task-id{color:#5eead4}
 .task-row .task-id{color:#38bdf8}
 .task-title{font-size:13px;color:#e2e8f0;line-height:1.5}
 .task-row .task-title{color:#cbd5e1;font-size:12.5px}
 /* .receipts：時間軸樣式——一條直線串起每一筆收據，每列左側一個圓點，
    比純色左邊框更看得出「這是一串按時間排列的紀錄」 */
 .receipts{position:relative;margin:4px 0 4px 5px;padding-left:18px;border-left:2px solid #1e293b}
 .receipt-row{position:relative;display:flex;gap:10px;align-items:center;padding:7px 0;margin:2px 0;font-size:12.5px;color:#cbd5e1}
 .receipt-row::before{content:'';position:absolute;left:-23px;top:50%;transform:translateY(-50%);width:9px;height:9px;border-radius:99px;background:#334155;border:2px solid #182238}
 .badge{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:10.5px;padding:3px 10px;border-radius:99px;white-space:nowrap;font-weight:700}
 .qa{background:#78350f33;border:1px solid #b4530966;border-radius:12px;padding:10px}
 .qa pre{white-space:pre-wrap;font-size:12px;margin:0 0 8px;font-family:inherit;max-height:320px;overflow-y:auto}
 textarea{width:100%;box-sizing:border-box;background:#0a0f1c;color:#e2e8f0;border:1px solid #475569;border-radius:8px;padding:8px;font-size:13px;min-height:60px}
 button{background:#334155;color:#e2e8f0;border:0;border-radius:9px;padding:7px 14px;font-size:12px;cursor:pointer;margin-top:6px}
 button:hover{background:#475569} button.primary{background:#4f46e5}
 .stopbtn{width:100%;background:transparent;border:1px solid #7f1d1d99;color:#fca5a5;border-radius:12px;padding:11px;font-size:13px;font-weight:600;cursor:pointer;margin-top:16px;display:flex;align-items:center;justify-content:center;gap:8px}
 .stopbtn:hover{background:#7f1d1d1a;border-color:#7f1d1d}
 .resumebtn{width:100%;background:transparent;border:1px solid #14532d99;color:#86efac;border-radius:12px;padding:11px;font-size:13px;font-weight:600;cursor:pointer;margin-top:16px;display:flex;align-items:center;justify-content:center;gap:8px}
 .resumebtn:hover{background:#14532d1a;border-color:#14532d}
 code{background:#334155;padding:2px 7px;border-radius:6px;font-size:12px;user-select:all}
 .ship{background:#064e3b55;border:1px solid #10b98155;border-radius:12px;padding:10px 12px;margin-top:10px;font-size:12.5px}
 .dirty{background:#78350f33;border:1px solid #b4530966;border-radius:12px;padding:10px 12px;margin-top:10px;font-size:12.5px;color:#fbbf24}
 .ricons{display:flex;gap:6px;flex-shrink:0}
 .dev-btn{display:inline-flex;align-items:center;justify-content:center;width:28px;height:28px;border:0;border-radius:9px;padding:0;margin:0;font-size:11px;cursor:pointer;flex-shrink:0}
 .dev-btn.start{background:#4f46e5;color:#fff} .dev-btn.start:hover{background:#4338ca}
 .dev-btn.stop{background:#1e293b;color:#f87171} .dev-btn.stop:hover{background:#334155}
 .icon-btn{display:inline-flex;align-items:center;justify-content:center;width:28px;height:28px;border-radius:9px;background:#1e293b;color:#38bdf8;flex-shrink:0}
 .icon-btn:hover{background:#334155;color:#7dd3fc}
 .icon-btn svg{width:14px;height:14px}
 .icon-btn.ph{background:transparent;pointer-events:none}
 .supform{background:#0d1526;border-radius:12px;padding:14px 16px;margin-top:12px}
 .supform .adv-toggle{background:transparent;color:#7dd3fc;border:0;padding:2px 0;margin:0 0 2px;font-size:12px;cursor:pointer}
 .supform .adv-toggle:hover{background:transparent;color:#38bdf8;text-decoration:underline}
 .supform .adv{margin-top:10px}
 .supform .caption{font-size:11px;color:#8b98ac;margin:0 0 10px}
 .supform .fields{display:flex;flex-wrap:wrap;gap:12px 22px;margin-bottom:12px}
 .supform .field{display:flex;flex-direction:column;gap:4px;color:#94a3b8;font-size:11.5px;white-space:nowrap}
 .supform select,.supform input[type=number]{background:#0a0f1c;color:#e2e8f0;border:1px solid #475569;border-radius:7px;padding:5px 8px;font-size:12.5px;width:100px;box-sizing:border-box;min-width:0}
 .supform .checks{display:flex;flex-wrap:wrap;gap:8px 20px;margin-bottom:10px}
 .supform label.chk{display:inline-flex;align-items:center;gap:6px;color:#cbd5e1;font-size:12px;white-space:nowrap}
 .supform .yolo-box{background:#7f1d1d1a;border:1px solid #7f1d1d55;border-radius:9px;padding:7px 10px;margin-bottom:12px}
 .supform .yolo-box label.chk{color:#fca5a5}
 .supform .cmdpreview{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:11.5px;background:#0a0f1c;border-radius:8px;padding:9px 11px;margin:2px 0 12px;overflow-x:auto;white-space:pre}
 .supform .cmdpreview .prompt{color:#64748b;margin-right:6px}
 .supform .cmdpreview code{background:transparent;padding:0;color:#cbd5e1}
 /* supervisor 啟動鍵刻意跟 dev server 的靛藍分開：這顆是真的開始跑 agent、
    花 quota，跟本機隨開隨關的 dev server 份量不同，借用狀態燈/任務卡/
    可出貨提示本來就在用的「有生產力」綠，不是另外發明一個新色。用低
    飽和的描邊 tonal 樣式而不是實心填色——這顆按鈕會出現在每一張閒置
    repo 卡片上，實心飽和色重複出現在多張卡片會太搶眼，描邊款維持
    「這是常態選項」的份量，不會像警報一樣喊 */
 .supform button.primary{background:#0596691a;border:1px solid #059669;color:#6ee7b7}
 .supform button.primary:hover{background:#05966933}
</style></head><body>
<h1>🤖 AI Engineer OS 控制台 <span class="muted" id="ts"></span> <span class="muted" id="usage"></span></h1>
<p class="muted" id="lan">{{LAN_INFO}}</p>
<p class="muted">panel 只讀寫協定檔（回答/煞車）；出貨與 merge 永遠在你的終端機。每 5 秒自動更新 · j/k 或 ↑↓ 切換 · Enter 展開/收合 · Space 收合</p>
<div class="grid" id="grid"></div>
<script>
const esc = s => (s||'').replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
// Feather 風格線條 icon（MIT），不外連字型/CDN——inline SVG 保持零外部依賴
const ICON_DASHBOARD='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"></line><line x1="12" y1="20" x2="12" y2="4"></line><line x1="6" y1="20" x2="6" y2="14"></line></svg>';
const ICON_OPEN='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="7" y1="17" x2="17" y2="7"></line><polyline points="7 7 17 7 17 17"></polyline></svg>';
const ICON_CLOCK='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>';
const ICON_CHEVRON='<svg class="chevron" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 6 15 12 9 18"></polyline></svg>';
async function post(url, data){ const b=new URLSearchParams(data);
  const r=await fetch(url,{method:'POST',body:b}); if(!r.ok) alert(await r.text()); refresh(); }
function stopRepo(repo,action){ post('/api/stop',{repo:repo,action:action}); }
// statusOf：分組＋狀態燈共用同一套判斷。paused 拆成兩種：真的等你回覆
// （paused）vs 已經回覆、只是還沒被下一輪 supervisor 消化（waiting）。
function statusOf(s){ if(s.missing) return 'missing';
  if(s.stopped) return 'stopped';
  if(s.paused && !s.paused_answered) return 'paused';
  if(s.supervisor_alive) return 'running';
  if(s.paused) return 'waiting';
  return 'idle'; }
const STATUS_ORDER=['paused','stopped','running','waiting','idle','missing'];
const STATUS_LABEL={paused:'❓ 需要你回覆',stopped:'🔴 已煞車',running:'🟢 執行中',
  waiting:'🔵 已回覆，待下一輪',idle:'⚪ 待命',missing:'⬜ 尚未 /ai-init'};
function splitId(s){ const i=(s||'').indexOf(' '); if(i<0) return [s||'','']; return [s.slice(0,i), s.slice(i+1)]; }
// rowMeta：收合列的一行摘要，依狀態挑最有資訊量的內容顯示。
function rowMeta(s){
  if(s.missing) return '尚未 /ai-init';
  if(s.stopped) return 'stopped · 第'+s.iteration+' 輪';
  if(s.paused && !s.paused_answered) return (s.paused_question||'').split('\n')[0];
  if(s.supervisor_alive) return (s.phase||'executing')+' · pid '+s.supervisor_pid+(s.current_task?' · '+splitId(s.current_task)[1]:'');
  if(s.paused) return '已回覆，待下一輪消化';
  if(s.current_task) return splitId(s.current_task)[1];
  return '待命 · 第'+s.iteration+' 輪'; }
// relTime：last_run_at（supervisor 本機時間，無時區字尾）轉相對時間，
// 純本機使用場景，跟瀏覽器同一時區，不用另外處理時區轉換。
function relTime(iso){
  if(!iso) return '';
  const t=new Date(iso.replace(' ','T'));
  if(isNaN(t.getTime())) return '';
  const m=Math.floor((Date.now()-t.getTime())/60000);
  if(m<1) return '剛剛';
  if(m<60) return m+'m';
  const h=Math.floor(m/60);
  if(h<24) return h+'h';
  return Math.floor(h/24)+'d'; }
const STATUS_COLORS = {paused:['#f59e0b','#fbbf24'],done:['#10b981','#34d399'],success:['#10b981','#34d399'],
  error:['#ef4444','#f87171'],failed:['#ef4444','#f87171'],rate_limit:['#f59e0b','#fbbf24'],no_status:['#475569','#94a3b8']};
function badge(status){ const c=STATUS_COLORS[status]||['#475569','#94a3b8'];
  return '<span class="badge" style="background:'+c[0]+'33;color:'+c[1]+'">'+esc(status)+'</span>'; }
function taskRow(cls,t){ const [id,title]=splitId(t);
  return '<div class="'+cls+'"><span class="task-id">'+esc(id)+'</span><span class="task-title">'+esc(title)+'</span></div>'; }
function receiptRow(r){ const m=r.match(/^(\S+)\s\[(\w+)\]\s(\[human\]\s)?([\s\S]*)$/);
  if(!m) return '<div class="receipt-row">'+esc(r)+'</div>';
  const src=m[3]?'<span class="badge" style="background:#8b5cf633;color:#a78bfa">human</span>':'';
  return '<div class="receipt-row">'+badge(m[2])+src+'<span>'+esc(m[1])+' · '+esc(m[4])+'</span></div>'; }
// supervisorBox：執行中顯示 pid＋log 連結（用下方既有的 STOP 按鈕煞車，
// 不重做一個停止鍵）；沒在跑、且 panel 有帶 -supervisor-script 才給啟動
// 表單——已煞車（stopped）時先隱藏，避免「啟動」跟「解除煞車」兩個按鈕
// 同時出現讓人搞不清楚順序（supervisor.sh 開跑會先看到 .ai/STOP 立刻退出）。
function supervisorBox(s){
  if(s.supervisor_alive){
    return '<div class="row">supervisor：<b style="color:#34d399">執行中</b>（pid '+s.supervisor_pid+'） '+
      '<a href="/api/supervisorlog?repo='+encodeURIComponent(s.path)+'" target="_blank" style="color:#e2e8f0;text-decoration:underline">log</a>'+
      ' <span class="muted">· 用下方 STOP 按鈕煞車</span></div>';
  }
  if(!s.supervisor_startable || s.stopped) return '';
  return '<div class="supform" data-repo="'+esc(s.path)+'">'+
    '<div class="section-label" style="margin-top:0">啟動 supervisor</div>'+
    '<button type="button" class="adv-toggle" data-act="advtoggle">進階設定 ▾</button>'+
    '<div class="adv" hidden>'+
    '<div class="caption">留白的欄位沿用 .ai/schedule.yml 的預設值（灰字為目前已知的預設值）</div>'+
    '<div class="fields">'+
    '<label class="field">model<select class="sf-model"><option value="">預設</option>'+
      '<option value="opus">opus</option><option value="sonnet">sonnet</option><option value="haiku">haiku</option></select></label>'+
    '<label class="field">quota-wait %<input class="sf-quota-wait" type="number" min="0" max="100" placeholder="60"></label>'+
    '<label class="field">max-iterations<input class="sf-max-iterations" type="number" min="1" placeholder="10"></label>'+
    '<label class="field">max-failures<input class="sf-max-failures" type="number" min="1" placeholder="3"></label>'+
    '</div>'+
    '<div class="checks">'+
    '<label class="chk"><input type="checkbox" class="sf-once">--once（只跑一輪）</label>'+
    '<label class="chk"><input type="checkbox" class="sf-review">--review（每任務後獨立審查）</label>'+
    '<label class="chk"><input type="checkbox" class="sf-wait-on-pause">--wait-on-pause（PAUSED 時輪詢不退出）</label>'+
    '</div>'+
    '<div class="yolo-box"><label class="chk"><input type="checkbox" class="sf-yolo">⚠ --yolo（跳過權限確認，只在信任的 repo 用）</label></div>'+
    '</div>'+
    '<div class="cmdpreview"><span class="prompt">$</span><code>supervisor.sh --repo '+esc(s.path)+'</code></div>'+
    '<button class="primary" data-act="supstart" data-repo="'+esc(s.path)+'">▶ 啟動 supervisor</button>'+
    '</div>'; }
// supArgsPreview／updateCmdPreview：表單目前的值即時組成等效指令字串，
// 跟 /api/supervisor 在 main.go 組 argv 的順序（model → quota-wait →
// max-iterations → max-failures → once/review/wait-on-pause/yolo）保持
// 一致，按鈕按下去實際送出的就是預覽看到的這行，不會兩邊兜不起來。
function qval(f,c){ const el=f.querySelector('.'+c); return el?el.value.trim():''; }
function qchk(f,c){ const el=f.querySelector('.'+c); return el?el.checked:false; }
function supArgsPreview(box){
  let cmd='supervisor.sh --repo '+box.dataset.repo;
  if(qval(box,'sf-model')) cmd+=' --model '+qval(box,'sf-model');
  if(qval(box,'sf-quota-wait')) cmd+=' --quota-wait '+qval(box,'sf-quota-wait');
  if(qval(box,'sf-max-iterations')) cmd+=' --max-iterations '+qval(box,'sf-max-iterations');
  if(qval(box,'sf-max-failures')) cmd+=' --max-failures '+qval(box,'sf-max-failures');
  if(qchk(box,'sf-once')) cmd+=' --once';
  if(qchk(box,'sf-review')) cmd+=' --review';
  if(qchk(box,'sf-wait-on-pause')) cmd+=' --wait-on-pause';
  if(qchk(box,'sf-yolo')) cmd+=' --yolo';
  return cmd; }
function updateCmdPreview(box){ const el=box.querySelector('.cmdpreview code'); if(el) el.textContent=supArgsPreview(box); }
// cardBody：展開後的完整內容（原本 card() 的全部細節），收合列只留
// dot/名稱/一行摘要/時間——細節要選中才付出畫面成本。
function cardBody(s){
  let h=supervisorBox(s);
  if(s.dev_command){
    if(s.dev_server_running){
      const who=s.dev_server_pid?'（pid '+s.dev_server_pid+'）':'（非 panel 啟動，無法追蹤 pid）';
      h+='<div class="row">dev server：<b style="color:#34d399">執行中</b>'+who+' '+
        '<a href="/api/devlog?repo='+encodeURIComponent(s.path)+'" target="_blank" style="color:#e2e8f0;text-decoration:underline">log</a> '+
        '<button data-act="devstop" data-repo="'+esc(s.path)+'">■ 停止</button></div>';
    } else h+='<div class="row">dev server：<b style="color:#8b98ac">未啟動</b> '+
      '<button class="primary" data-act="devstart" data-repo="'+esc(s.path)+'">▶ 啟動</button></div>';
  }
  if(s.last_run_status) h+='<div class="row">上輪 '+esc(s.last_run_status)+' $'+esc(s.last_run_cost||'0')+'</div>';
  const total=s.backlog_count+s.done_count, pct=total>0?Math.round(s.done_count/total*100):0;
  h+='<div class="stats"><span>待辦 '+s.backlog_count+' · 完成 '+s.done_count+'</span>'+
     '<span class="bar"><i style="width:'+pct+'%"></i></span><span class="pct">'+pct+'%</span></div>';
  if(s.current_task){ h+='<div class="section-label">進行中</div>'+taskRow('task-card',s.current_task); }
  h+='<div class="section-label">待辦 '+s.backlog_count+' / 完成 '+s.done_count+'</div>';
  if((s.backlog||[]).length) h+=s.backlog.map(t=>taskRow('task-row',t)).join('');
  if((s.receipts||[]).length){ h+='<div class="section-label">最近收據</div>'+
    '<div class="receipts">'+s.receipts.map(receiptRow).join('')+'</div>'; }
  if(s.paused && !s.paused_answered){
    h+='<div class="qa" data-repo="'+esc(s.path)+'"><b>❓ agent 的問題</b><pre>'+esc(s.paused_question)+'</pre>'+
      '<textarea placeholder="你的決定（會附寫進 PAUSED，下一輪 agent 自行路由）"></textarea>'+
      '<button class="primary" data-act="answer" data-repo="'+esc(s.path)+'">送出回覆</button></div>'; }
  else if(s.paused && s.paused_answered){
    h+='<div class="dirty">✓ 問題已回覆，但不會自動觸發——需要你手動跑一輪：'+
       '<br><code>supervisor/supervisor.sh --repo '+esc(s.path)+' --once</code>'+
       '<br>下次想跳過這步：用 <code>--wait-on-pause</code>（或 schedule.yml 設 '+
       '<code>wait_on_pause: true</code>）跑，撞到 PAUSED 不會退出，回覆後它自己 5 分鐘內接著跑</div>'; }
  if(s.dirty_count>0){
    h+='<div class="dirty">⚠ working tree 有 '+s.dirty_count+' 個未 commit 檔案 —— 未記帳的工作，'+
       '互動 session 收尾記得跑 <code>/ai-wrap</code></div>'; }
  if(s.shippable>0){
    h+='<div class="ship">🚢 ai/queue 領先 '+s.shippable+' 個 commit，可出貨：'+
       '<br><code>claude</code> 內執行 <code>/ai-ship '+esc(s.path)+'</code></div>'; }
  if(s.stopped) h+='<button class="resumebtn" data-act="resume" data-repo="'+esc(s.path)+'">'+
    '<span class="dot" style="background:currentColor"></span>解除煞車</button>';
  else h+='<button class="stopbtn" data-act="stop" data-repo="'+esc(s.path)+'">'+
    '<span class="dot" style="background:currentColor"></span>STOP 煞車</button>';
  return h; }
// slot：固定位置的按鈕格——某個 repo 沒支援這個動作時，補一個同尺寸的
// 隱形佔位（.ph），讓每一行的按鈕都對在同一欄，不會因為這個 repo 少了
// 某個按鈕就讓後面的按鈕跟著往左飄。
function slot(html){ return html || '<span class="icon-btn ph"></span>'; }
// rowIcons：儀表板／排程／dev url 三個連結按鈕，永遠在收合列右側同一個
// 位置（不用展開才點得到）。是 <a target="_blank">，點擊要讓瀏覽器自己
// 開新分頁，不觸發收合列的展開/收合——見下面 click delegation 的例外判斷。
function rowIcons(s){
  return '<span class="ricons">'+
    slot(s.dashboard_ready?'<a class="icon-btn" href="/dashboard?repo='+encodeURIComponent(s.path)+
     '" target="_blank" title="儀表板">'+ICON_DASHBOARD+'</a>':'')+
    slot(s.schedule_ready?'<a class="icon-btn" href="/api/schedule?repo='+encodeURIComponent(s.path)+
     '" target="_blank" title="排程設定（.ai/schedule.yml）">'+ICON_CLOCK+'</a>':'')+
    slot(s.dev_url?'<a class="icon-btn" href="'+esc(s.dev_url)+
     '" target="_blank" title="'+esc(s.dev_url)+'">'+ICON_OPEN+'</a>':'')+
   '</span>'; }
// rowDevBtn：收合列上的 ▶/■ 快速鍵，跟展開內容裡那份是同一個
// devstart/devstop 動作——展開版多了 pid 與 log 連結，這裡只求不用展開
// 就能一鍵切換，兩者共存不衝突（都只是 POST /api/devserver）。
function rowDevBtn(s){
  return slot(!s.dev_command ? '' : s.dev_server_running
    ? '<button class="dev-btn stop" data-act="devstop" data-repo="'+esc(s.path)+'" title="停止 dev server'+(s.dev_server_pid?'（pid '+s.dev_server_pid+'）':'（非 panel 啟動，無法追蹤 pid）')+'">■</button>'
    : '<button class="dev-btn start" data-act="devstart" data-repo="'+esc(s.path)+'" title="啟動 dev server">▶</button>'); }
function item(s){
  const st=statusOf(s), expanded=s.path===expandedRepo;
  let h='<div class="repo'+(expanded?' expanded':'')+'" data-repo="'+esc(s.path)+'">'+
    '<div class="rrow" data-repo="'+esc(s.path)+'"><span class="dot '+st+'"></span>'+
    '<span class="rname">'+esc(s.name)+'</span>'+
    '<span class="rmeta">'+esc(rowMeta(s))+'</span>'+
    (s.missing?'':'<span class="rstats">待辦 '+s.backlog_count+' · 完成 '+s.done_count+'</span>')+
    rowDevBtn(s)+rowIcons(s)+
    '<span class="rtime">'+esc(relTime(s.last_run_at))+'</span>'+ICON_CHEVRON+'</div>';
  if(expanded) h+='<div class="rbody">'+(s.missing?'<p class="muted">尚未 /ai-init</p>':cardBody(s))+'</div>';
  h+='</div>';
  return h; }
function saveQaState(){
  // 展開/收合、5 秒重繪都會砍掉 DOM，連帶重置 <pre> 捲動位置與 textarea
  // 打到一半的字——重繪前先存、重繪後照 repo 路徑復原。
  const out={};
  document.querySelectorAll('.qa[data-repo]').forEach(qa=>{
    const repo=qa.dataset.repo, pre=qa.querySelector('pre'), ta=qa.querySelector('textarea');
    out[repo]={scrollTop:pre?pre.scrollTop:0, text:ta?ta.value:'', focused:ta===document.activeElement,
      selStart:ta?ta.selectionStart:0, selEnd:ta?ta.selectionEnd:0};
  });
  return out; }
function restoreQaState(saved){
  document.querySelectorAll('.qa[data-repo]').forEach(qa=>{
    const st=saved[qa.dataset.repo]; if(!st) return;
    const pre=qa.querySelector('pre'), ta=qa.querySelector('textarea');
    if(pre) pre.scrollTop=st.scrollTop;
    if(ta && st.text){ ta.value=st.text;
      if(st.focused){ ta.focus(); ta.setSelectionRange(st.selStart,st.selEnd); } }
  }); }
// saveSupState／restoreSupState：跟 qa 的 textarea 一樣的問題——5 秒重繪
// 會砍掉 DOM，連帶收合「進階設定」、清空正在填的表單值。存/復原展開
// 狀態、8 個欄位值、目前 focus 在哪個欄位（用 class 名記，欄位不會重複）。
function saveSupState(){
  const out={};
  document.querySelectorAll('.supform[data-repo]').forEach(f=>{
    const adv=f.querySelector('.adv'), active=document.activeElement;
    out[f.dataset.repo]={
      adv: adv?!adv.hidden:false,
      model:qval(f,'sf-model'), quota_wait:qval(f,'sf-quota-wait'),
      max_iterations:qval(f,'sf-max-iterations'), max_failures:qval(f,'sf-max-failures'),
      once:qchk(f,'sf-once'), review:qchk(f,'sf-review'),
      wait_on_pause:qchk(f,'sf-wait-on-pause'), yolo:qchk(f,'sf-yolo'),
      focused:(active && f.contains(active)) ? active.className : null,
    };
  });
  return out; }
function restoreSupState(saved){
  document.querySelectorAll('.supform[data-repo]').forEach(f=>{
    const st=saved[f.dataset.repo]; if(!st) return;
    const adv=f.querySelector('.adv'), toggle=f.querySelector('.adv-toggle');
    if(adv && st.adv){ adv.hidden=false; if(toggle) toggle.textContent='進階設定 ▴'; }
    const setv=(c,v)=>{ const el=f.querySelector('.'+c); if(el && v) el.value=v; };
    const setc=(c,v)=>{ const el=f.querySelector('.'+c); if(el) el.checked=v; };
    setv('sf-model',st.model); setv('sf-quota-wait',st.quota_wait);
    setv('sf-max-iterations',st.max_iterations); setv('sf-max-failures',st.max_failures);
    setc('sf-once',st.once); setc('sf-review',st.review);
    setc('sf-wait-on-pause',st.wait_on_pause); setc('sf-yolo',st.yolo);
    updateCmdPreview(f);
    if(st.focused){ const el=f.querySelector('.'+st.focused); if(el) el.focus(); }
  }); }
function groupHeader(key,count){
  return '<div class="status-header"><span class="dot '+key+'"></span>'+STATUS_LABEL[key]+
    ' <span class="count">'+count+'</span></div>'; }
// 收合列表 + 單一展開（accordion）：focusedRepo 是鍵盤焦點（藍框），
// expandedRepo 是目前展開內容的那一個——兩者分開，j/k 掃描時內容不會
// 一直跳開跳合，Enter 才真的展開/收合。兩者都存 repo path 不存 index，
// 因為分組會隨狀態變動增減，index 撐不住。autoExpandDone 只在第一次
// 拿到資料時，把最急迫分組的第一筆自動展開，之後尊重使用者手動選擇。
let focusedRepo=null, expandedRepo=null, autoExpandDone=false, renderOrder=[], lastList=[];
function applyFocusHighlight(){
  document.querySelectorAll('.repo[data-repo]').forEach(el=>{
    el.classList.toggle('focused', el.dataset.repo===focusedRepo); }); }
function moveFocus(delta){
  if(renderOrder.length===0) return;
  let idx=renderOrder.indexOf(focusedRepo);
  idx = idx<0 ? (delta>0?0:renderOrder.length-1) : Math.min(Math.max(idx+delta,0),renderOrder.length-1);
  focusedRepo=renderOrder[idx];
  applyFocusHighlight();
  document.querySelector('.repo[data-repo="'+CSS.escape(focusedRepo)+'"]')
    ?.scrollIntoView({block:'nearest',behavior:'smooth'}); }
function toggleExpand(path){
  expandedRepo = expandedRepo===path ? null : path;
  focusedRepo = path;
  renderList(); }
document.addEventListener('keydown', e=>{
  const active=document.activeElement, tag=active&&active.tagName;
  if(tag==='TEXTAREA'){
    if((e.metaKey||e.ctrlKey) && e.key==='Enter'){
      e.preventDefault();
      const btn=active.closest('.qa')?.querySelector('button[data-act="answer"]');
      if(btn) btn.click();
    }
    return;
  }
  if(tag==='BUTTON'||tag==='A'||tag==='INPUT') return; // 讓原生鍵盤操作（Tab+Enter 按鈕）照常運作
  if(e.key==='ArrowDown'||e.key==='j'){ e.preventDefault(); moveFocus(1); }
  else if(e.key==='ArrowUp'||e.key==='k'){ e.preventDefault(); moveFocus(-1); }
  else if(e.key==='Enter' && focusedRepo){
    e.preventDefault();
    toggleExpand(focusedRepo);
    const el=document.querySelector('.repo[data-repo="'+CSS.escape(focusedRepo)+'"]');
    if(expandedRepo===focusedRepo){
      const ta=el && el.querySelector('.qa textarea');
      if(ta) ta.focus(); else el?.scrollIntoView({block:'center',behavior:'smooth'});
    }
  } else if(e.key===' ' && expandedRepo){ e.preventDefault(); expandedRepo=null; renderList(); }
});
function renderList(){
  const saved=saveQaState(), savedSup=saveSupState();
  const groups={}; STATUS_ORDER.forEach(k=>groups[k]=[]);
  lastList.forEach(s=>groups[statusOf(s)].push(s));
  // renderOrder／expandedRepo 要在組 html 字串之前算好——item() 讀
  // expandedRepo 決定要不要印展開內容，晚算的話第一次自動展開只會
  // 更新到變數，畫面要等下一次重繪（5 秒後）才追上，剛載入頁面會
  // 有一拍是「明明選了最急迫的卡片，畫面卻是收合的」。
  renderOrder = STATUS_ORDER.flatMap(k=>groups[k].map(s=>s.path));
  if(!autoExpandDone && renderOrder.length){ expandedRepo=renderOrder[0]; focusedRepo=renderOrder[0]; autoExpandDone=true; }
  if(expandedRepo && !renderOrder.includes(expandedRepo)) expandedRepo=null;
  let html='';
  STATUS_ORDER.forEach(k=>{ if(groups[k].length===0) return;
    html+=groupHeader(k,groups[k].length)+groups[k].map(item).join(''); });
  document.getElementById('grid').innerHTML=html;
  restoreQaState(saved);
  restoreSupState(savedSup);
  applyFocusHighlight(); }
async function refresh(){
  try{ const r=await fetch('/api/state'); lastList=await r.json();
    renderList();
    document.getElementById('ts').textContent='更新於 '+new Date().toLocaleTimeString();
  }catch(e){ document.getElementById('ts').textContent='更新失敗：'+e; } }
function usagePct(label,pct){ if(pct<0) return '';
  const color=pct>=80?'#f87171':pct>=60?'#fbbf24':'#94a3b8';
  return label+' <b style="color:'+color+'">'+pct+'%</b>'; }
async function refreshUsage(){
  try{ const r=await fetch('/api/usage'); const u=await r.json();
    const el=document.getElementById('usage');
    if(u.error){ el.textContent='用量：'+u.error; return; }
    el.innerHTML='· 用量 '+[usagePct('5h',u.session_pct),usagePct('7d',u.week_pct)].filter(Boolean).join(' ');
  }catch(e){} }
// 事件委派：按鈕/收合列的 repo 路徑放 data- 屬性（inline onclick 塞含
// 引號的路徑字串會截斷 HTML 屬性——按鈕全滅的前科），重繪也不用重綁
// handler。button[data-act] 優先判斷，避免點展開內容裡的按鈕誤觸收合列
// 的展開/收合（兩者是兄弟節點，不會互相 closest() 到，這裡順序純粹是
// 防禦性寫法）。
document.getElementById('grid').addEventListener('click', e=>{
  if(e.target.closest('a.icon-btn')) return; // 讓連結自己開新分頁，不觸發收合列展開/收合
  const b=e.target.closest('button[data-act]');
  if(b){
    const repo=b.dataset.repo;
    if(b.dataset.act==='advtoggle'){
      const box=b.closest('.supform'), adv=box.querySelector('.adv');
      adv.hidden=!adv.hidden;
      b.textContent = adv.hidden ? '進階設定 ▾' : '進階設定 ▴';
    } else if(b.dataset.act==='answer'){
      const t=b.closest('.qa').querySelector('textarea').value;
      if(t.trim()) post('/api/answer',{repo:repo,text:t});
    } else if(b.dataset.act==='devstart'||b.dataset.act==='devstop'){
      post('/api/devserver',{repo:repo,action:b.dataset.act==='devstart'?'start':'stop'});
    } else if(b.dataset.act==='supstart'){
      const box=b.closest('.supform');
      if(qchk(box,'sf-yolo') && !confirm('確定要用 --yolo（跳過權限確認）啟動嗎？只在信任的 repo 用。')) return;
      post('/api/supervisor',{repo:repo,action:'start',model:qval(box,'sf-model'),
        quota_wait:qval(box,'sf-quota-wait'),max_iterations:qval(box,'sf-max-iterations'),max_failures:qval(box,'sf-max-failures'),
        once:qchk(box,'sf-once')?'1':'',review:qchk(box,'sf-review')?'1':'',
        wait_on_pause:qchk(box,'sf-wait-on-pause')?'1':'',yolo:qchk(box,'sf-yolo')?'1':''});
    } else stopRepo(repo,b.dataset.act);
    return;
  }
  const row=e.target.closest('.rrow');
  if(row) toggleExpand(row.dataset.repo);
});
// 啟動 supervisor 表單任何欄位一改，等效指令預覽跟著更新——input 蓋數字/
// 下拉即時輸入，change 蓋 checkbox 勾選（兩個都掛，選字/勾選都會觸發，
// updateCmdPreview 本身是全量重算，重複觸發沒有副作用）。
['input','change'].forEach(evt=>document.getElementById('grid').addEventListener(evt, e=>{
  const box=e.target.closest('.supform');
  if(box) updateCmdPreview(box);
}));
refresh(); setInterval(refresh, 5000);
refreshUsage(); setInterval(refreshUsage, 60000);
</script></body></html>`
