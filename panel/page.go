package main

// pageHTML：單頁控制台。每 5 秒輪詢 /api/state 重繪；
// 動作只有兩種 POST（回答 PAUSED、STOP/恢復），出貨顯示指令供複製。
const pageHTML = `<!DOCTYPE html><html lang="zh-Hant"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AI Engineer OS — 控制台</title>
<style>
 body{font-family:-apple-system,'PingFang TC',sans-serif;background:#0f172a;color:#e2e8f0;margin:0;padding:20px;max-width:1100px;margin:auto}
 h1{font-size:18px} .muted{color:#64748b;font-size:12px}
 .grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(330px,1fr));gap:14px;margin-top:14px}
 .repo{background:#1e293b;border:1px solid #334155;border-radius:14px;padding:16px}
 .repo h2{font-size:15px;margin:0 0 6px;display:flex;align-items:center;gap:8px}
 .dot{width:9px;height:9px;border-radius:99px;display:inline-block}
 .running{background:#34d399}.idle{background:#64748b}.stopped{background:#ef4444}.paused{background:#f59e0b}
 .row{font-size:13px;margin:3px 0} .k{color:#94a3b8}
 ul{margin:4px 0;padding-left:18px;font-size:12px;color:#cbd5e1} li{margin:2px 0}
 .qa{background:#78350f33;border:1px solid #b4530966;border-radius:10px;padding:10px;margin-top:8px}
 .qa pre{white-space:pre-wrap;font-size:12px;margin:0 0 8px;font-family:inherit}
 textarea{width:100%;box-sizing:border-box;background:#0f172a;color:#e2e8f0;border:1px solid #475569;border-radius:8px;padding:8px;font-size:13px;min-height:60px}
 button{background:#334155;color:#e2e8f0;border:0;border-radius:8px;padding:6px 12px;font-size:12px;cursor:pointer;margin-top:6px}
 button:hover{background:#475569} button.primary{background:#4f46e5} button.danger{background:#7f1d1d}
 code{background:#334155;padding:2px 6px;border-radius:5px;font-size:12px;user-select:all}
 .ship{background:#064e3b55;border:1px solid #10b98155;border-radius:10px;padding:8px 10px;margin-top:8px;font-size:12px}
</style></head><body>
<h1>🤖 AI Engineer OS 控制台 <span class="muted" id="ts"></span></h1>
<p class="muted">panel 只讀寫協定檔（回答/煞車）；出貨與 merge 永遠在你的終端機。每 5 秒自動更新。</p>
<div class="grid" id="grid"></div>
<script>
const esc = s => (s||'').replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
async function post(url, data){ const b=new URLSearchParams(data);
  const r=await fetch(url,{method:'POST',body:b}); if(!r.ok) alert(await r.text()); refresh(); }
function answer(repo){ const t=document.getElementById('ans-'+CSS.escape(repo)).value;
  if(t.trim()) post('/api/answer',{repo:repo,text:t}); }
function stopRepo(repo,action){ post('/api/stop',{repo:repo,action:action}); }
function dot(s){ if(s.stopped) return ['stopped','已煞車'];
  if(s.paused) return ['paused', s.paused_answered ? '已回答，待下一輪消化' : '等你回答'];
  if(s.supervisor_alive) return ['running','supervisor 執行中 pid '+s.supervisor_pid];
  return ['idle','待命']; }
function card(s){
  if(s.missing) return '<div class="repo"><h2>'+esc(s.name)+'</h2><p class="muted">尚未 /ai-init</p></div>';
  const [cls,label]=dot(s);
  let h='<div class="repo"><h2><span class="dot '+cls+'"></span>'+esc(s.name)+' <span class="muted">'+label+'</span></h2>';
  h+='<div class="row"><span class="k">狀態</span> '+esc(s.phase||'?')+'（第 '+s.iteration+' 輪）';
  if(s.last_run_status) h+='｜上輪 '+esc(s.last_run_status)+' $'+esc(s.last_run_cost||'0');
  h+='</div>';
  if(s.current_task) h+='<div class="row"><span class="k">進行中</span> '+esc(s.current_task)+'</div>';
  h+='<div class="row"><span class="k">待辦 '+s.backlog_count+'</span>／<span class="k">完成 '+s.done_count+'</span></div>';
  if((s.backlog||[]).length) h+='<ul>'+s.backlog.map(t=>'<li>'+esc(t)+'</li>').join('')+'</ul>';
  if((s.receipts||[]).length) h+='<div class="row k">最近收據</div><ul>'+s.receipts.map(t=>'<li>'+esc(t)+'</li>').join('')+'</ul>';
  if(s.paused){
    h+='<div class="qa"><b>❓ agent 的問題</b><pre>'+esc(s.paused_question)+'</pre>';
    if(s.paused_answered){ h+='<span class="muted">已有回覆，下一輪 /work 會消化並繼續。</span>'; }
    else { h+='<textarea id="ans-'+esc(s.path)+'" placeholder="你的決定（會附寫進 PAUSED，下一輪 agent 自行路由）"></textarea>'+
      '<button class="primary" onclick="answer('+JSON.stringify(s.path)+')">送出回覆</button>'; }
    h+='</div>'; }
  if(s.shippable>0){
    h+='<div class="ship">🚢 ai/queue 領先 '+s.shippable+' 個 commit，可出貨：'+
       '<br><code>claude</code> 內執行 <code>/ai-ship '+esc(s.path)+'</code></div>'; }
  h+='<div>';
  if(s.stopped) h+='<button onclick="stopRepo('+JSON.stringify(s.path)+',\'resume\')">解除煞車</button>';
  else h+='<button class="danger" onclick="stopRepo('+JSON.stringify(s.path)+',\'stop\')">STOP 煞車</button>';
  h+='</div></div>'; return h; }
async function refresh(){
  try{ const r=await fetch('/api/state'); const list=await r.json();
    document.getElementById('grid').innerHTML=list.map(card).join('');
    document.getElementById('ts').textContent='更新於 '+new Date().toLocaleTimeString();
  }catch(e){ document.getElementById('ts').textContent='更新失敗：'+e; } }
refresh(); setInterval(refresh, 5000);
</script></body></html>`
