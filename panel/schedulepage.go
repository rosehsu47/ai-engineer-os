package main

import "fmt"

// scheduleFieldsHTML：欄位清單本身（scheduleFields）不分 repo，只算一次
// 就好——真正的「目前值」由頁面自己的 JS fetch /api/schedule-config 填進
// 這裡先產生好的空殼 input。分類標題跟著 scheduleFields 的 Category 換
// 就插一個新的 section 標題，順序即 scheduleFields 定義順序。
func scheduleFieldsHTML() string {
	out, lastCat := "", ""
	for _, f := range scheduleFields {
		if f.Category != lastCat {
			if lastCat != "" {
				out += "</div>"
			}
			out += `<div class="section-label">` + f.Category + `</div><div class="fgroup">`
			lastCat = f.Category
		}
		switch f.Kind {
		case "bool":
			out += fmt.Sprintf(`<label class="field chk"><input type="checkbox" data-key="%s" data-kind="bool"><span>%s</span></label>`, f.Key, f.Label)
		case "model":
			out += fmt.Sprintf(`<label class="field"><span>%s</span><select data-key="%s" data-kind="model">`+
				`<option value="opus">opus</option><option value="sonnet">sonnet</option><option value="haiku">haiku</option></select></label>`,
				f.Label, f.Key)
		case "text":
			out += fmt.Sprintf(`<label class="field wide"><span>%s</span><input type="text" data-key="%s" data-kind="text" placeholder="留白＝不設定"></label>`, f.Label, f.Key)
		default: // int
			out += fmt.Sprintf(`<label class="field"><span>%s</span><input type="number" data-key="%s" data-kind="int"></label>`, f.Label, f.Key)
		}
		out += fmt.Sprintf(`<div class="fhelp">%s</div>`, f.Help)
	}
	if lastCat != "" {
		out += "</div>"
	}
	return out
}

var scheduleFormHTML = `<!DOCTYPE html><html lang="zh-Hant"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>排程設定 — AI Engineer OS</title>
<style>
 body{font-family:-apple-system,'PingFang TC',sans-serif;background:#0a0f1c;color:#e2e8f0;margin:0 auto;padding:24px 28px 60px;max-width:720px}
 h1{font-size:17px;margin:0 0 4px}
 .muted{color:#8b98ac;font-size:12.5px}
 .section-label{font-size:14px;font-weight:700;color:#f1f5f9;margin:26px 0 10px}
 .fgroup{display:flex;flex-direction:column;gap:14px}
 .field{display:flex;flex-direction:column;gap:5px;font-size:12.5px;color:#94a3b8}
 .field.wide input{width:100%;box-sizing:border-box}
 .field.chk{flex-direction:row;align-items:center;gap:8px;color:#cbd5e1;font-size:13px}
 .field input[type=number],.field input[type=text],.field select{background:#182238;color:#e2e8f0;border:1px solid #334155;border-radius:8px;padding:7px 10px;font-size:13px;width:160px;box-sizing:border-box;font-family:inherit}
 .field input[type=text]{width:100%;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
 .field input:focus,.field select:focus{outline:2px solid #4f46e5;outline-offset:1px}
 .fhelp{font-size:11.5px;color:#64748b;margin-top:-8px}
 .abtn{display:inline-flex;align-items:center;justify-content:center;gap:6px;height:36px;padding:0 18px;border-radius:9px;font-size:13px;font-weight:600;cursor:pointer;border:1px solid transparent;box-sizing:border-box}
 .abtn.primary{background:#4f46e5;color:#fff;border-color:#4f46e5}
 .abtn.primary:hover{background:#4338ca}
 .abtn.outline{background:transparent;color:#8b98ac;border-color:#334155}
 .abtn.outline:hover{border-color:#475569;color:#cbd5e1}
 .bar{position:sticky;bottom:0;background:#0a0f1cee;backdrop-filter:blur(6px);border-top:1px solid #263047;padding:14px 0;margin-top:30px;display:flex;align-items:center;gap:12px}
 .msg{font-size:12.5px}
 .msg.ok{color:#34d399} .msg.err{color:#f87171}
 code{background:#182238;padding:2px 7px;border-radius:6px;font-size:12px}
</style></head><body>
<h1>排程設定</h1>
<p class="muted">repo：<code>{{REPO_TEXT}}</code> · 改的是 <code>.ai/schedule.yml</code>（supervisor.sh 只讀，改完立即生效，不用重啟任何東西）
 · <a href="/api/schedule?repo={{REPO_URL}}" target="_blank" style="color:#7dd3fc">查看原始檔案</a></p>
<form id="f">` + scheduleFieldsHTML() + `
<div class="bar">
  <button type="submit" class="abtn primary">儲存</button>
  <button type="button" class="abtn outline" id="reload">重新載入目前值</button>
  <span class="msg" id="msg"></span>
</div>
</form>
<script>
const repo=new URLSearchParams(location.search).get('repo')||'';
function fieldEls(){ return document.querySelectorAll('[data-key]'); }
async function load(){
  const msg=document.getElementById('msg'); msg.textContent='載入中…'; msg.className='msg';
  try{
    const r=await fetch('/api/schedule-config?repo='+encodeURIComponent(repo));
    if(!r.ok) throw new Error(await r.text());
    const vals=await r.json();
    fieldEls().forEach(el=>{
      const k=el.dataset.key, kind=el.dataset.kind, v=vals[k];
      if(kind==='bool') el.checked = v==='true';
      else el.value = v||'';
    });
    msg.textContent=''; msg.className='msg';
  }catch(e){ msg.textContent='讀取失敗：'+e; msg.className='msg err'; }
}
document.getElementById('reload').addEventListener('click', load);
document.getElementById('f').addEventListener('submit', async e=>{
  e.preventDefault();
  const msg=document.getElementById('msg'); msg.textContent='儲存中…'; msg.className='msg';
  const body=new URLSearchParams(); body.set('repo', repo);
  fieldEls().forEach(el=>{
    const k=el.dataset.key, kind=el.dataset.kind;
    body.set(k, kind==='bool' ? (el.checked?'true':'false') : el.value);
  });
  try{
    const r=await fetch('/api/schedule-config',{method:'POST',body});
    if(!r.ok) throw new Error(await r.text());
    msg.textContent='已存檔'; msg.className='msg ok';
  }catch(e){ msg.textContent='存檔失敗：'+e; msg.className='msg err'; }
});
load();
</script>
</body></html>`
