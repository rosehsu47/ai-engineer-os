package main

// pageHTML：單頁控制台。每 5 秒輪詢 /api/state 重繪；
// 動作只有兩種 POST（回答 PAUSED、STOP/恢復），出貨顯示指令供複製。
const pageHTML = `<!DOCTYPE html><html lang="zh-Hant"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AI Engineer OS — 控制台</title>
<style>
 body{font-family:-apple-system,'PingFang TC',sans-serif;background:#0a0f1c;color:#e2e8f0;margin:0 auto;padding:20px 28px;max-width:1720px}
 h1{font-size:18px} .muted{color:#64748b;font-size:12px}
 .grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(min(480px,100%),1fr));gap:16px;margin-top:14px}
 .repo{background:#141b2d;border:1px solid #263047;border-radius:16px;padding:18px;display:flex;flex-direction:column}
 .repo h2{font-size:15px;margin:0 0 4px;display:flex;align-items:center;gap:8px;flex-wrap:wrap}
 .name{font-weight:700;color:#f1f5f9;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
 .meta{font-size:12px;color:#64748b;font-weight:400}
 .dot{width:8px;height:8px;border-radius:99px;display:inline-block;flex-shrink:0}
 .running{background:#34d399}.idle{background:#64748b}.stopped{background:#ef4444}.paused{background:#f59e0b}
 .row{font-size:12px;margin:4px 0;color:#94a3b8}
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
 .stopbtn{width:100%;background:transparent;border:1px solid #7f1d1d;color:#f87171;border-radius:10px;padding:10px;font-size:13px;font-weight:600;cursor:pointer;margin-top:auto}
 .stopbtn:hover{background:#7f1d1d26}
 .resumebtn{width:100%;background:transparent;border:1px solid #14532d;color:#4ade80;border-radius:10px;padding:10px;font-size:13px;font-weight:600;cursor:pointer;margin-top:auto}
 .resumebtn:hover{background:#14532d26}
 code{background:#334155;padding:2px 6px;border-radius:5px;font-size:12px;user-select:all}
 .ship{background:#064e3b55;border:1px solid #10b98155;border-radius:10px;padding:8px 10px;margin-top:8px;font-size:12px}
 .dirty{background:#78350f33;border:1px solid #b4530966;border-radius:10px;padding:8px 10px;margin-top:8px;font-size:12px;color:#fbbf24}
</style></head><body>
<h1>🤖 AI Engineer OS 控制台 <span class="muted" id="ts"></span> <span class="muted" id="usage"></span></h1>
<p class="muted">panel 只讀寫協定檔（回答/煞車）；出貨與 merge 永遠在你的終端機。每 5 秒自動更新。</p>
<div class="grid" id="grid"></div>
<script>
const esc = s => (s||'').replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
async function post(url, data){ const b=new URLSearchParams(data);
  const r=await fetch(url,{method:'POST',body:b}); if(!r.ok) alert(await r.text()); refresh(); }
function stopRepo(repo,action){ post('/api/stop',{repo:repo,action:action}); }
function dot(s){ if(s.stopped) return 'stopped';
  if(s.paused && !s.paused_answered) return 'paused';
  if(s.supervisor_alive) return 'running';
  if(s.paused) return 'paused';
  return 'idle'; }
function metaLine(s){
  const parts=[];
  if(s.stopped){ parts.push('<b style="color:#f87171">stopped</b>'); }
  else if(s.paused && !s.paused_answered){ parts.push('<b style="color:#fbbf24">paused · 待回覆</b>'); }
  else if(s.supervisor_alive){ parts.push('supervisor'); parts.push('<b style="color:#34d399">'+esc(s.phase||'executing')+'</b>'); parts.push('pid '+s.supervisor_pid); }
  else if(s.paused){ parts.push('<b style="color:#fbbf24">paused · 已回覆，待下一輪消化</b>'); }
  else { parts.push('<b style="color:#94a3b8">idle</b>'); }
  parts.push('第 '+s.iteration+' 輪');
  return parts.join(' · '); }
function splitId(s){ const i=(s||'').indexOf(' '); if(i<0) return [s||'','']; return [s.slice(0,i), s.slice(i+1)]; }
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
function card(s){
  if(s.missing) return '<div class="repo"><h2><span class="name">'+esc(s.name)+'</span></h2><p class="muted">尚未 /ai-init</p></div>';
  let h='<div class="repo"><h2><span class="dot '+dot(s)+'"></span><span class="name">'+esc(s.name)+'</span>'+
    '<span class="meta">'+metaLine(s)+'</span></h2>';
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
       '<br><code>supervisor/supervisor.sh --repo '+esc(s.path)+' --once</code></div>'; }
  if(s.dirty_count>0){
    h+='<div class="dirty">⚠ working tree 有 '+s.dirty_count+' 個未 commit 檔案 —— 未記帳的工作，'+
       '互動 session 收尾記得跑 <code>/ai-wrap</code></div>'; }
  if(s.shippable>0){
    h+='<div class="ship">🚢 ai/queue 領先 '+s.shippable+' 個 commit，可出貨：'+
       '<br><code>claude</code> 內執行 <code>/ai-ship '+esc(s.path)+'</code></div>'; }
  if(s.stopped) h+='<button class="resumebtn" data-act="resume" data-repo="'+esc(s.path)+'">解除煞車</button>';
  else h+='<button class="stopbtn" data-act="stop" data-repo="'+esc(s.path)+'">■ STOP 煞車</button>';
  h+='</div>'; return h; }
function saveQaState(){
  // 每 5 秒的整格重繪會砍掉 DOM，連帶重置 <pre> 捲動位置與 textarea
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
async function refresh(){
  try{ const r=await fetch('/api/state'); const list=await r.json();
    const saved=saveQaState();
    document.getElementById('grid').innerHTML=list.map(card).join('');
    restoreQaState(saved);
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
// 事件委派：按鈕的 repo 路徑放 data- 屬性（inline onclick 塞含引號的
// 路徑字串會截斷 HTML 屬性——按鈕全滅的前科），重繪也不用重綁 handler
document.getElementById('grid').addEventListener('click', e=>{
  const b=e.target.closest('button[data-act]'); if(!b) return;
  const repo=b.dataset.repo;
  if(b.dataset.act==='answer'){
    const t=b.closest('.qa').querySelector('textarea').value;
    if(t.trim()) post('/api/answer',{repo:repo,text:t});
  } else stopRepo(repo,b.dataset.act);
});
refresh(); setInterval(refresh, 5000);
refreshUsage(); setInterval(refreshUsage, 60000);
</script></body></html>`
