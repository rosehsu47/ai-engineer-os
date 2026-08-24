package main

// pageHTML：單頁控制台。每 5 秒輪詢 /api/state 重繪；
// 動作只有兩種 POST（回答 PAUSED、STOP/恢復），出貨顯示指令供複製。
const pageHTML = `<!DOCTYPE html><html lang="zh-Hant"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AI Engineer OS — 控制台</title>
<style>
 body{font-family:-apple-system,'PingFang TC',sans-serif;background:#0a0f1c;color:#e2e8f0;margin:0 auto;padding:20px 28px;max-width:1100px}
 h1{font-size:18px} .muted{color:#64748b;font-size:12px}
 .grid{display:flex;flex-direction:column;gap:6px;margin-top:14px}
 .repo{background:#141b2d;border:1px solid #263047;border-radius:12px}
 .repo.focused{border-color:#38bdf8;box-shadow:0 0 0 2px #38bdf855}
 .rrow{display:flex;align-items:center;gap:10px;padding:11px 16px;cursor:pointer}
 .rrow:hover{background:#1a2338}
 .rrow .rname{font-weight:700;color:#f1f5f9;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;flex-shrink:0}
 .rrow .rmeta{color:#94a3b8;font-size:12.5px;flex:1;min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
 .rrow .rstats{color:#64748b;font-size:11.5px;flex-shrink:0;font-variant-numeric:tabular-nums}
 .rrow .rtime{color:#64748b;font-size:11.5px;flex-shrink:0;font-variant-numeric:tabular-nums;width:2.5em;text-align:right}
 .rrow .chevron{color:#475569;flex-shrink:0;transition:transform .15s}
 .repo.expanded .chevron{transform:rotate(90deg)}
 .rbody{padding:2px 18px 18px;border-top:1px solid #1e293b;display:flex;flex-direction:column}
 .dot{width:8px;height:8px;border-radius:99px;display:inline-block;flex-shrink:0}
 .running{background:#34d399}.idle{background:#64748b}.stopped{background:#ef4444}.paused{background:#f59e0b}.waiting{background:#38bdf8}.missing{background:#334155}
 .status-header{font-size:13px;font-weight:700;color:#e2e8f0;margin:18px 0 6px;padding-bottom:6px;border-bottom:1px solid #263047;display:flex;align-items:center;gap:8px}
 .status-header:first-child{margin-top:0}
 .status-header .count{font-weight:400;color:#64748b;font-size:12px}
 .row{font-size:12px;margin:10px 0 4px;color:#94a3b8}
 .stats{display:flex;align-items:center;gap:10px;font-size:12px;color:#94a3b8;margin:10px 0}
 .stats .bar{flex:1;height:4px;background:#1e293b;border-radius:99px;overflow:hidden}
 .stats .bar i{display:block;height:100%;background:#34d399}
 .stats .pct{color:#cbd5e1;font-variant-numeric:tabular-nums;font-weight:600}
 .section-label{font-size:11px;color:#64748b;margin:14px 0 6px}
 .task-card{display:flex;gap:12px;align-items:flex-start;background:#0f1c30;border-left:4px solid #34d399;border-radius:10px;padding:12px 14px;margin:6px 0}
 .task-row{display:flex;gap:12px;align-items:flex-start;border-left:3px solid #38bdf866;padding:8px 12px;margin:6px 0}
 .task-id{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;white-space:nowrap;padding-top:2px}
 .task-card .task-id{color:#5eead4}
 .task-row .task-id{color:#38bdf8}
 .task-title{font-size:13px;color:#e2e8f0;line-height:1.5}
 .task-row .task-title{color:#cbd5e1;font-size:12.5px}
 .receipt-row{display:flex;gap:10px;align-items:center;border-left:3px solid #f59e0b66;padding:8px 12px;margin:6px 0;font-size:12px;color:#cbd5e1}
 .badge{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:10.5px;padding:2px 9px;border-radius:99px;white-space:nowrap;font-weight:700}
 .qa{background:#78350f33;border:1px solid #b4530966;border-radius:10px;padding:10px;margin-top:8px}
 .qa pre{white-space:pre-wrap;font-size:12px;margin:0 0 8px;font-family:inherit;max-height:320px;overflow-y:auto}
 textarea{width:100%;box-sizing:border-box;background:#0a0f1c;color:#e2e8f0;border:1px solid #475569;border-radius:8px;padding:8px;font-size:13px;min-height:60px}
 button{background:#334155;color:#e2e8f0;border:0;border-radius:8px;padding:6px 12px;font-size:12px;cursor:pointer;margin-top:6px}
 button:hover{background:#475569} button.primary{background:#4f46e5}
 .stopbtn{width:100%;background:transparent;border:1px solid #7f1d1d;color:#f87171;border-radius:10px;padding:10px;font-size:13px;font-weight:600;cursor:pointer;margin-top:12px}
 .stopbtn:hover{background:#7f1d1d26}
 .resumebtn{width:100%;background:transparent;border:1px solid #14532d;color:#4ade80;border-radius:10px;padding:10px;font-size:13px;font-weight:600;cursor:pointer;margin-top:12px}
 .resumebtn:hover{background:#14532d26}
 code{background:#334155;padding:2px 6px;border-radius:5px;font-size:12px;user-select:all}
 .ship{background:#064e3b55;border:1px solid #10b98155;border-radius:10px;padding:8px 10px;margin-top:8px;font-size:12px}
 .dirty{background:#78350f33;border:1px solid #b4530966;border-radius:10px;padding:8px 10px;margin-top:8px;font-size:12px;color:#fbbf24}
 .ricons{display:flex;gap:4px;flex-shrink:0}
 .dev-btn{display:inline-flex;align-items:center;justify-content:center;width:24px;height:24px;border:0;border-radius:7px;padding:0;margin:0;font-size:11px;cursor:pointer;flex-shrink:0}
 .dev-btn.start{background:#4f46e5;color:#fff} .dev-btn.start:hover{background:#4338ca}
 .dev-btn.stop{background:#1e293b;color:#f87171} .dev-btn.stop:hover{background:#334155}
 .icon-btn{display:inline-flex;align-items:center;justify-content:center;width:24px;height:24px;border-radius:7px;background:#1e293b;color:#38bdf8;flex-shrink:0}
 .icon-btn:hover{background:#334155;color:#7dd3fc}
 .icon-btn svg{width:14px;height:14px}
 .icon-btn.ph{background:transparent;pointer-events:none}
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
// cardBody：展開後的完整內容（原本 card() 的全部細節），收合列只留
// dot/名稱/一行摘要/時間——細節要選中才付出畫面成本。
function cardBody(s){
  let h='';
  if(s.dev_command){
    if(s.dev_server_running){
      const who=s.dev_server_pid?'（pid '+s.dev_server_pid+'）':'（非 panel 啟動，無法追蹤 pid）';
      h+='<div class="row">dev server：<b style="color:#34d399">執行中</b>'+who+' '+
        '<a href="/api/devlog?repo='+encodeURIComponent(s.path)+'" target="_blank" style="color:#e2e8f0;text-decoration:underline">log</a> '+
        '<button data-act="devstop" data-repo="'+esc(s.path)+'">■ 停止</button></div>';
    } else h+='<div class="row">dev server：<b style="color:#64748b">未啟動</b> '+
      '<button class="primary" data-act="devstart" data-repo="'+esc(s.path)+'">▶ 啟動</button></div>';
  }
  if(s.last_run_status) h+='<div class="row">上輪 '+esc(s.last_run_status)+' $'+esc(s.last_run_cost||'0')+'</div>';
  const total=s.backlog_count+s.done_count, pct=total>0?Math.round(s.done_count/total*100):0;
  h+='<div class="stats"><span>待辦 '+s.backlog_count+' · 完成 '+s.done_count+'</span>'+
     '<span class="bar"><i style="width:'+pct+'%"></i></span><span class="pct">'+pct+'%</span></div>';
  if(s.current_task){ h+='<div class="section-label">進行中</div>'+taskRow('task-card',s.current_task); }
  h+='<div class="section-label">待辦 '+s.backlog_count+' / 完成 '+s.done_count+'</div>';
  if((s.backlog||[]).length) h+=s.backlog.map(t=>taskRow('task-row',t)).join('');
  if((s.receipts||[]).length){ h+='<div class="section-label">最近收據</div>'+s.receipts.map(receiptRow).join(''); }
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
  if(s.stopped) h+='<button class="resumebtn" data-act="resume" data-repo="'+esc(s.path)+'">解除煞車</button>';
  else h+='<button class="stopbtn" data-act="stop" data-repo="'+esc(s.path)+'">■ STOP 煞車</button>';
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
  const saved=saveQaState();
  const groups={}; STATUS_ORDER.forEach(k=>groups[k]=[]);
  lastList.forEach(s=>groups[statusOf(s)].push(s));
  let html='';
  STATUS_ORDER.forEach(k=>{ if(groups[k].length===0) return;
    html+=groupHeader(k,groups[k].length)+groups[k].map(item).join(''); });
  renderOrder = STATUS_ORDER.flatMap(k=>groups[k].map(s=>s.path));
  if(!autoExpandDone && renderOrder.length){ expandedRepo=renderOrder[0]; focusedRepo=renderOrder[0]; autoExpandDone=true; }
  if(expandedRepo && !renderOrder.includes(expandedRepo)) expandedRepo=null;
  document.getElementById('grid').innerHTML=html;
  restoreQaState(saved);
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
    if(b.dataset.act==='answer'){
      const t=b.closest('.qa').querySelector('textarea').value;
      if(t.trim()) post('/api/answer',{repo:repo,text:t});
    } else if(b.dataset.act==='devstart'||b.dataset.act==='devstop'){
      post('/api/devserver',{repo:repo,action:b.dataset.act==='devstart'?'start':'stop'});
    } else stopRepo(repo,b.dataset.act);
    return;
  }
  const row=e.target.closest('.rrow');
  if(row) toggleExpand(row.dataset.repo);
});
refresh(); setInterval(refresh, 5000);
refreshUsage(); setInterval(refreshUsage, 60000);
</script></body></html>`
