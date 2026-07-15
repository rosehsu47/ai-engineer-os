#!/usr/bin/env bash
# dashboard.sh — 把 .ai/ 狀態渲染成單檔靜態 HTML（零額度、零依賴）
# 用法：dashboard.sh --repo /path/to/repo [--out 檔名]
#       產出預設在 {repo}/.ai/reports/dashboard.html，瀏覽器直接開
set -u

REPO="" OUT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --repo) REPO="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    *) echo "unknown flag: $1" >&2; exit 64 ;;
  esac
done
[ -n "$REPO" ] && [ -d "$REPO/.ai" ] || { echo "--repo 必填且需含 .ai/" >&2; exit 66; }
OUT="${OUT:-$REPO/.ai/reports/dashboard.html}"
mkdir -p "$(dirname "$OUT")"

esc() { sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g'; }
jget() { # jget <file> <key>  （字串或數字，扁平 JSON）
  grep -oE "\"$2\"[[:space:]]*:[[:space:]]*(\"[^\"]*\"|[0-9.]+|null|true|false)" "$1" 2>/dev/null \
    | head -1 | sed -E 's/^[^:]+:[[:space:]]*//; s/^"//; s/"$//'
}

CKPT="$REPO/.ai/state/checkpoint.json"
LAST="$REPO/.ai/supervisor/last_run.json"
phase=$(jget "$CKPT" phase); phase=${phase:-?}
iteration=$(jget "$CKPT" iteration); iteration=${iteration:-0}
last_task=$(jget "$CKPT" last_completed_task_id)
run_status=$(jget "$LAST" last_status)
run_cost=$(jget "$LAST" total_cost_usd)
run_at=$(jget "$LAST" at)

count_tasks() { grep -c '^  - id:' "$1" 2>/dev/null || echo 0; }
n_backlog=$(count_tasks "$REPO/.ai/tasks/backlog.yaml")
n_doing=$(count_tasks "$REPO/.ai/tasks/doing.yaml")
n_done=$(count_tasks "$REPO/.ai/tasks/done.yaml")

stop="🟢 執行中/待命"
[ -f "$REPO/.ai/STOP" ] && stop="🔴 STOP（手動煞車中）"
[ -f "$REPO/.ai/PAUSED" ] && stop="🟡 PAUSED（等待人類：$(head -1 "$REPO/.ai/PAUSED" 2>/dev/null | esc)）"

# receipts 表（最近 15 張，讀 frontmatter）
receipt_rows=$(
  find "$REPO/.ai/receipts" -name '*.md' -type f 2>/dev/null | sort -r | head -15 | while read -r f; do
    fm=$(awk '/^---$/{n++; next} n==1{print} n>=2{exit}' "$f")
    get() { printf '%s\n' "$fm" | grep -E "^$1:" | head -1 | sed -E "s/^$1:[[:space:]]*//; s/^\"//; s/\"$//"; }
    tid=$(get task_id); title=$(get title); st=$(get status); com=$(get commit)
    score=$(printf '%s\n' "$fm" | grep -oE 'score:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | head -1)
    tests=$(printf '%s\n' "$fm" | grep -oE 'result:[[:space:]]*"?[a-z]+' | head -1 | grep -oE '[a-z]+$')
    review="—"; grep -q '## 獨立審查' "$f" && review=$(grep -A2 '## 獨立審查' "$f" | grep -oE 'PASS|FAIL' | head -1)
    # case pattern 加前括號：在 $() 內未配對的 ')' 會讓 bash 解析錯亂
    badge="s-other"; case "$st" in (done) badge="s-done";; (failed|blocked) badge="s-bad";; (partial|paused) badge="s-warn";; esac
    printf '<tr><td>%s</td><td>%s</td><td><span class="badge %s">%s</span></td><td>%s</td><td>%s</td><td>%s</td><td><code>%s</code></td></tr>\n' \
      "$(basename "$(dirname "$f")")/$(basename "$f" .md)" \
      "$(printf '%s' "$title" | esc)" "$badge" "$st" "${score:-—}" "$review" "${tests:-—}" "${com:-—}"
  done
)

# git 事件（ai/queue 最近 12 個 commit）
git_rows=$(
  git -C "$REPO" log ai/queue --oneline -12 2>/dev/null | while read -r line; do
    printf '<tr><td><code>%s</code></td><td>%s</td></tr>\n' \
      "${line%% *}" "$(printf '%s' "${line#* }" | esc)"
  done
)

# supervisor 事件（loop 層遙測，events.jsonl 最近 10 行；檔案不存在則空）
event_rows=$(
  tail -10 "$REPO/.ai/supervisor/events.jsonl" 2>/dev/null | while read -r line; do
    jline() { printf '%s' "$line" | grep -oE "\"$1\":\"[^\"]*\"" | head -1 | sed -E 's/^"[^"]+":"//; s/"$//'; }
    at=$(jline at); ev=$(jline event); detail=$(jline detail)
    it=$(printf '%s' "$line" | grep -oE '"iter":[0-9]+' | grep -oE '[0-9]+')
    printf '<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n' \
      "$(printf '%s' "$at" | esc)" "$(printf '%s' "$ev" | esc)" "${it:-—}" "$(printf '%s' "$detail" | esc)"
  done
)

# done 任務表
done_rows=$(
  awk '/^  - id:/{id=$3} /^    title:/{sub(/^    title:[ ]*/,""); t=$0} /^    result:/{r=$2}
       /^    receipt:/{sub(/^    receipt:[ ]*/,""); gsub(/"/,""); printf "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", id, t, r, $0}' \
    "$REPO/.ai/tasks/done.yaml" 2>/dev/null
)

cat > "$OUT" <<HTML
<!DOCTYPE html><html lang="zh-Hant"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AI Engineer OS — $(basename "$REPO")</title>
<style>
  body{font-family:-apple-system,'PingFang TC',sans-serif;background:#0f172a;color:#e2e8f0;margin:0;padding:24px;max-width:960px;margin:auto}
  h1{font-size:20px} h2{font-size:15px;color:#94a3b8;margin-top:28px}
  .cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px}
  .card{background:#1e293b;border:1px solid #334155;border-radius:12px;padding:14px}
  .card .v{font-size:22px;font-weight:700} .card .k{font-size:12px;color:#94a3b8;margin-top:4px}
  table{width:100%;border-collapse:collapse;font-size:13px;margin-top:8px}
  th,td{text-align:left;padding:6px 8px;border-bottom:1px solid #334155} th{color:#94a3b8;font-weight:500}
  code{background:#334155;padding:1px 5px;border-radius:4px;font-size:12px}
  .badge{padding:1px 8px;border-radius:99px;font-size:12px}
  .s-done{background:#065f46} .s-bad{background:#7f1d1d} .s-warn{background:#78350f} .s-other{background:#334155}
  .muted{color:#64748b;font-size:12px}
</style></head><body>
<h1>🤖 AI Engineer OS — $(basename "$REPO")</h1>
<p class="muted">狀態：$stop ｜ 產生於 $(date '+%Y-%m-%d %H:%M')（重新產生：\`dashboard.sh --repo ...\`）</p>
<div class="cards">
  <div class="card"><div class="v">$n_backlog / $n_doing / $n_done</div><div class="k">backlog / doing / done</div></div>
  <div class="card"><div class="v">$phase</div><div class="k">checkpoint phase（第 $iteration 輪）</div></div>
  <div class="card"><div class="v">${last_task:-—}</div><div class="k">最後完成任務</div></div>
  <div class="card"><div class="v">\$${run_cost:-0}</div><div class="k">上次 run 成本（${run_status:-尚未跑過}｜${run_at:-—}）</div></div>
</div>
<h2>📋 Receipts（最近 15）</h2>
<table><tr><th>收據</th><th>任務</th><th>狀態</th><th>自評</th><th>獨立審查</th><th>測試</th><th>commit</th></tr>
${receipt_rows:-<tr><td colspan=7 class=muted>還沒有收據</td></tr>}</table>
<h2>✅ 已完成任務</h2>
<table><tr><th>ID</th><th>標題</th><th>結果</th><th>收據</th></tr>
${done_rows:-<tr><td colspan=4 class=muted>還沒有完成的任務</td></tr>}</table>
<h2>🌿 Git 事件（ai/queue 最近 12 個 commit）</h2>
<table><tr><th>sha</th><th>訊息</th></tr>
${git_rows:-<tr><td colspan=2 class=muted>ai/queue 分支尚無 commit</td></tr>}</table>
<h2>⚙️ Supervisor 事件（events.jsonl 最近 10）</h2>
<table><tr><th>時間</th><th>事件</th><th>輪</th><th>細節</th></tr>
${event_rows:-<tr><td colspan=4 class=muted>還沒有 loop 事件</td></tr>}</table>
</body></html>
HTML
echo "dashboard: $OUT"
