#!/usr/bin/env bash
# supervisor.sh — AI Engineer OS 的無人監督迴圈
#
# 每輪開一個全新的 `claude -p "/work"` session（狀態都在 .ai/ 檔案裡，
# 不用 --resume），讀取 AIOS_STATUS 與錯誤徵兆做分類與復原。
# 協定見 AI-RUNTIME.md；錯誤分類表見本檔 classify()。
#
# 用法：
#   supervisor.sh --repo /path/to/repo [--once] [--max-iterations N]
#                 [--max-failures N] [--model M] [--claude-flags "..."]
#                 [--yolo] [--wait-on-pause] [--dry-run] [--verbose]
#   supervisor.sh --self-test        # 零額度：用內嵌 fixtures 驗證分類器
#   supervisor.sh --doctor --repo X  # 零額度環境體檢（首跑前在一般終端機執行）
#   supervisor.sh --doctor --probe --repo X  # 體檢＋headless 寫入實測（花少量額度）
#
# 安全閥：單 repo 單 supervisor（lock）、max_iterations、連續失敗上限、
# 每輪 watchdog timeout、run 累計成本熔斷（max_cost_per_run_usd）、
# .ai/STOP 隨時手動煞車。macOS bash 3.2 相容。
set -u

# ---------- 參數與預設 ----------
REPO="" ONCE=0 YOLO=0 WAIT_ON_PAUSE=0 DRY_RUN=0 VERBOSE=0 SELF_TEST=0 REVIEW=""
DOCTOR=0 PROBE=0
MAX_ITER="" MAX_FAIL="" MODEL="" EXTRA_FLAGS=""

while [ $# -gt 0 ]; do
  case "$1" in
    --repo) REPO="$2"; shift 2 ;;
    --once) ONCE=1; shift ;;
    --max-iterations) MAX_ITER="$2"; shift 2 ;;
    --max-failures) MAX_FAIL="$2"; shift 2 ;;
    --model) MODEL="$2"; shift 2 ;;
    --claude-flags) EXTRA_FLAGS="$2"; shift 2 ;;
    --yolo) YOLO=1; shift ;;
    --review) REVIEW=true; shift ;;
    --wait-on-pause) WAIT_ON_PAUSE=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    --self-test) SELF_TEST=1; shift ;;
    --doctor) DOCTOR=1; shift ;;
    --probe) PROBE=1; shift ;;
    --verbose) VERBOSE=1; shift ;;
    *) echo "unknown flag: $1" >&2; exit 64 ;;
  esac
done

log() {
  local line; line="$(date '+%Y-%m-%dT%H:%M:%S') $*"
  echo "$line" >&2
  [ -n "${SUP_DIR:-}" ] && echo "$line" >> "$SUP_DIR/run.log"
  return 0
}
vlog() { [ "$VERBOSE" = 1 ] && log "$@"; return 0; }

# ---------- schedule.yml 讀取（扁平 key） ----------
sched_get() { # sched_get key default
  local v=""
  [ -f "$SCHED" ] && v=$(grep -E "^$1:" "$SCHED" 2>/dev/null | head -1 \
    | sed -E 's/^[^:]+:[[:space:]]*//; s/[[:space:]]*#.*$//; s/^"(.*)"$/\1/; s/[[:space:]]*$//')
  [ -n "$v" ] && printf '%s' "$v" || printf '%s' "$2"
}

# ---------- 輸出解析（jq 有則用、無則 grep/sed 降級） ----------
HAVE_JQ=0; command -v jq >/dev/null 2>&1 && HAVE_JQ=1

extract_result() { # stdin: claude 的 --output-format json 輸出 → result 文字
  if [ "$HAVE_JQ" = 1 ]; then jq -r '.result // empty' 2>/dev/null
  else sed -n 's/.*"result"[[:space:]]*:[[:space:]]*"\(.*\)".*/\1/p' | head -1; fi
}
extract_cost() { # stdin → total_cost_usd（無則 0）
  local c=""
  if [ "$HAVE_JQ" = 1 ]; then c=$(jq -r '.total_cost_usd // empty' 2>/dev/null)
  else c=$(grep -oE '"total_cost_usd"[[:space:]]*:[[:space:]]*[0-9.]+' | grep -oE '[0-9.]+' | head -1); fi
  [ -n "$c" ] && printf '%s' "$c" || printf '0'
}
extract_is_error() {
  if [ "$HAVE_JQ" = 1 ]; then jq -r '.is_error // false' 2>/dev/null
  else grep -q '"is_error"[[:space:]]*:[[:space:]]*true' && echo true || echo false; fi
}

# ---------- loop 層事件（機械發出 → .ai/supervisor/events.jsonl；LLM 不參與） ----------
# 事件 schema 見 AI-RUNTIME.md「事件模型」。值一律消毒（有損 by design，原文在 run.log）。
emit_event() { # emit_event <event> [detail]；iter/class/status_tok/task_tok/cost 取當前迴圈變數
  [ -n "${SUP_DIR:-}" ] || return 0
  local ev="$1" detail san_d san_s san_t
  detail="${2:-}"
  san_d=$(printf '%s' "$detail" | tr -d '"\\' | tr '\n\t' '  ' | cut -c1-200)
  san_s=$(printf '%s' "${status_tok:-}" | tr -cd 'A-Z_')
  san_t=$(printf '%s' "${task_tok:-none}" | tr -cd 'A-Za-z0-9_.-' | cut -c1-32)
  printf '{"at":"%s","event":"%s","iter":%s,"class":"%s","status":"%s","task":"%s","cost_usd":%s,"detail":"%s"}\n' \
    "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$ev" "${iter:-0}" "${class:-}" "$san_s" "${san_t:-none}" "${cost:-0}" "$san_d" \
    >> "$SUP_DIR/events.jsonl"
  return 0
}

# ---------- 錯誤分類（由上而下，首個命中） ----------
# classify <exit_code> <combined_output_file> → 印出類別字串
classify() {
  local ec="$1" f="$2" all status_line
  all=$(cat "$f" 2>/dev/null || true)
  status_line=$(printf '%s\n' "$all" | grep -oE 'AIOS_STATUS: [A-Z_]+' | tail -1 | awk '{print $2}')

  case "${status_line:-}" in
    STOPPED)      echo stopped; return ;;
    PAUSED)       echo paused; return ;;
    QUEUE_EMPTY)  echo queue_empty; return ;;
    DONE_TASK|TASK_PARTIAL|BLOCKED|CONTRACT_HALT) echo productive; return ;;
  esac
  # 改這個 regex 前，先在 run_self_test 加上該訊息「原文」的 fixture——
  # CLI 的 limit 文案變體很多，fixture 先紅再改才知道沒弄壞舊變體
  if printf '%s' "$all" | grep -qiE 'hit your [a-z ]*limit|(usage|session|weekly) limit|limit reached|rate.?limit|(^|[^0-9])429([^0-9]|$)|resets( at)? [0-9]{1,2}(:[0-9]{2})?[[:space:]]?(am|pm)'; then
    echo rate_limit; return
  fi
  if printf '%s' "$all" | grep -qiE 'overloaded|529|ECONNRESET|ETIMEDOUT|ENOTFOUND|fetch failed|socket hang|temporarily unavailable'; then
    echo network; return
  fi
  case "$ec" in 124|137|143) echo killed; return ;; esac
  if [ "$ec" != 0 ] || printf '%s' "$all" | grep -q '"is_error"[[:space:]]*:[[:space:]]*true'; then
    echo crash; return
  fi
  echo no_status   # exit 0 但沒有 AIOS_STATUS 行：協定漂移
}

# ---------- rate-limit 睡眠（epoch 形優先，再試 "resets 8pm"，都失敗則固定 fallback） ----------
sleep_until_reset() { # $1 = 全部輸出文字
  local ts hour min ampm target now cap epoch
  now=$(date +%s)
  # headless CLI 最機器可讀的形式：「…limit reached|<unix epoch>」→ 直接睡到 epoch
  epoch=$(printf '%s' "$1" | grep -oiE 'limit reached\|[0-9]{9,}' | head -1 | grep -oE '[0-9]{9,}')
  if [ -n "${epoch:-}" ]; then
    target=$((epoch + 120))                          # reset 後多等 2 分鐘
    cap=$((now + 21600))                             # 上限 6 小時
    [ "$target" -gt "$cap" ] && target=$cap
    [ "$target" -le "$now" ] && target=$((now + 120))  # epoch 已過：至少等 2 分鐘再試
    log "rate limit: 睡到 $(date -r "$target" '+%H:%M')（reset epoch ${epoch}）"
    do_sleep $((target - now)); return
  fi
  ts=$(printf '%s' "$1" | grep -oiE 'resets( at)? [0-9]{1,2}(:[0-9]{2})?[[:space:]]?(am|pm)' | head -1)
  hour=$(printf '%s' "$ts" | grep -oE '[0-9]{1,2}' | head -1)
  min=$(printf '%s' "$ts" | grep -oE ':[0-9]{2}' | head -1 | tr -d ':')
  ampm=$(printf '%s' "$ts" | grep -oiE '(am|pm)' | tail -1 | tr 'A-Z' 'a-z')
  if [ -n "${hour:-}" ] && [ -n "${ampm:-}" ]; then
    target=$(date -j -f '%Y-%m-%d %I:%M%p' "$(date '+%Y-%m-%d') ${hour}:${min:-00}${ampm}" +%s 2>/dev/null || echo "")
    if [ -n "$target" ]; then
      [ "$target" -le "$now" ] && target=$((target + 86400))
      target=$((target + 120))                       # reset 後多等 2 分鐘
      cap=$((now + 21600))                           # 上限 6 小時
      [ "$target" -gt "$cap" ] && target=$cap
      log "rate limit: 睡到 $(date -r "$target" '+%H:%M')（resets ${hour}:${min:-00}${ampm}）"
      do_sleep $((target - now)); return
    fi
  fi
  log "rate limit: 無法解析 reset 時間，固定睡 ${RL_FALLBACK_MIN} 分鐘"
  do_sleep $((RL_FALLBACK_MIN * 60))
}

do_sleep() { # 可被 AIOS_FAKE_SLEEP=1 假化（self-test 用）
  if [ "${AIOS_FAKE_SLEEP:-0}" = 1 ]; then echo "FAKE_SLEEP ${1}s"; else sleep "$1"; fi
}

# ---------- quota 煞車（5h / 7d 用量達門檻即停，保留個人額度） ----------
# parse_usage_pct <claude /usage 的 result 文字> <session|week> → 百分比數字或空字串
# 純文字解析，與 API 呼叫分離以便 self-test 覆蓋。
parse_usage_pct() {
  local text="$1" label="$2" line
  case "$label" in
    session) line=$(printf '%s' "$text" | grep -m1 -E 'Current session') ;;
    week)    line=$(printf '%s' "$text" | grep -m1 -E 'Current week') ;;
  esac
  printf '%s' "$line" | grep -oE '[0-9]+%' | head -1 | tr -d '%'
}

# quota_decide <sess%> <week%> <wait門檻> <stop門檻> → stop|wait|go
# 純函式（self-test 直接覆蓋）。規則：
# - 硬門檻：只看 7d → stop（寫 .ai/STOP，保個人額度；7d 要等數天不值得等）
# - 軟門檻：只看 5h，門檻以上（含 100%）一律 wait，不 stop——
#   5h 會 reset，值得等，等到降回門檻下才繼續（wait 迴圈另有 24h 逾時保險）
# - 查不到用量（空值）→ go，不誤殺
quota_decide() {
  local sess="${1:-}" week="${2:-}" wait_t="$3" stop_t="$4"
  if [ -n "$week" ] && [ "$week" -ge "$stop_t" ] 2>/dev/null; then echo stop; return; fi
  if [ "$wait_t" -gt 0 ] 2>/dev/null && [ -n "$sess" ] && [ "$sess" -ge "$wait_t" ] 2>/dev/null; then
    echo wait; return
  fi
  echo go
}

# quota_check：查 5h/7d 用量。7d 硬門檻寫 .ai/STOP 回傳 1；5h 達軟門檻
#（含超過硬門檻、100%）在函式內等待（每 recheck 分鐘查一次 /usage，
# 零額度）直到降回門檻下才回傳 0——任務只在額度足以整輪跑完時才開工，
# 不會中途斷頭浪費重讀。
quota_check() {
  local stop_t wait_t recheck out result sess week verdict reason waited_min=0
  stop_t=$(sched_get quota_stop_threshold_pct 80)
  wait_t=$(sched_get quota_wait_threshold_pct 60)
  recheck=$(sched_get quota_wait_recheck_minutes 20)
  while :; do
    out=$( (cd "$REPO" && claude -p "/usage" --output-format json) 2>/dev/null )
    [ -z "$out" ] && return 0
    result=$(printf '%s' "$out" | extract_result)
    [ -z "$result" ] && return 0
    sess=$(parse_usage_pct "$result" session)
    week=$(parse_usage_pct "$result" week)
    verdict=$(quota_decide "$sess" "$week" "$wait_t" "$stop_t")
    case "$verdict" in
      go) return 0 ;;
      stop)
        reason="7d 已用 ${week:-?}%（硬門檻 ${stop_t}%，5h 已用 ${sess:-?}%）"
        log "quota 煞車：$reason —— 保留給個人使用"
        printf 'quota 煞車（%s）\n%s\n調整門檻：.ai/schedule.yml 的 quota_stop_threshold_pct\n解除：刪除本檔或按 panel 的「解除煞車」\n' \
          "$(date '+%Y-%m-%dT%H:%M:%S')" "$reason" > "$REPO/.ai/STOP"
        END_REASON=quota_stop; emit_event quota_stop "$reason"
        return 1 ;;
      wait)
        if [ "$waited_min" -ge 1440 ]; then
          log "quota 軟門檻等了 24h 仍未降（5h=${sess}%——個人使用持續占用？），寫 STOP 收工"
          printf 'quota 軟門檻等待逾 24h（%s）\n5h 用量持續 ≥ %s%%\n' \
            "$(date '+%Y-%m-%dT%H:%M:%S')" "$wait_t" > "$REPO/.ai/STOP"
          END_REASON=quota_wait_timeout; emit_event quota_stop "軟門檻等待逾 24h（5h=${sess}%）"
          return 1
        fi
        log "quota 軟門檻：5h 已用 ${sess}%（≥${wait_t}%），不開新任務，${recheck} 分鐘後再查（已等 ${waited_min} 分）"
        emit_event quota_wait "5h=${sess}% >= ${wait_t}%，已等 ${waited_min} 分"
        do_sleep $((recheck * 60)); waited_min=$((waited_min + recheck))
        [ -f "$REPO/.ai/STOP" ] && { log "等待期間發現 .ai/STOP，結束"; return 1; } ;;
    esac
  done
}

# ---------- 狀態檔結構 lint（只偵測，不修復——修復永遠歸 /work 的協定自癒） ----------
TAB_CHAR=$(printf '\t')

lint_checkpoint() { # <file> → 印 ok 或壞掉原因；壞掉 return 1
  local f="$1" first
  [ -s "$f" ] || { echo "檔案不存在或為空"; return 1; }
  if [ "$HAVE_JQ" = 1 ]; then
    jq -e 'type=="object" and has("phase")' "$f" >/dev/null 2>&1 \
      || { echo "非合法 JSON object 或缺 phase key"; return 1; }
  else
    first=$(tr -d '[:space:]' < "$f" | cut -c1)
    [ "$first" = "{" ] || { echo "首字元不是 {（無 jq 淺檢查）"; return 1; }
    grep -q '"phase"' "$f" || { echo "缺 phase key（無 jq 淺檢查）"; return 1; }
  fi
  echo ok
}

lint_tasks() { # <file> <backlog|doing|done> → 印 ok 或壞掉原因；壞掉 return 1
  local f="$1" kind="$2" n bad
  [ -f "$f" ] || { echo "檔案不存在"; return 1; }
  grep -q '^version:' "$f" || { echo "缺 version: key"; return 1; }
  grep -q '^tasks:' "$f"   || { echo "缺 tasks: key"; return 1; }
  if grep -q "$TAB_CHAR" "$f"; then echo "含 tab 字元（YAML 不合法）"; return 1; fi
  bad=$(grep -E '^[[:space:]]*- id:' "$f" | grep -cvE 'id:[[:space:]]*"?T-[0-9]+"?')
  [ "$bad" = 0 ] || { echo "${bad} 個 id 不是 T-NNN 格式"; return 1; }
  if [ "$kind" = doing ]; then
    n=$(grep -cE '^[[:space:]]*- id:' "$f")
    [ "$n" -le 1 ] || { echo "doing 有 ${n} 筆任務（不變量：至多 1）"; return 1; }
  fi
  echo ok
}

lint_state() { # 檢查 $REPO 的四個狀態檔；每個異常 log 一行，echo 異常數
  local bad=0 r k
  r=$(lint_checkpoint "$REPO/.ai/state/checkpoint.json")
  [ "$r" = ok ] || { log "state lint: checkpoint.json — $r"; bad=$((bad+1)); }
  for k in backlog doing done; do
    r=$(lint_tasks "$REPO/.ai/tasks/$k.yaml" "$k")
    [ "$r" = ok ] || { log "state lint: ${k}.yaml — $r"; bad=$((bad+1)); }
  done
  echo "$bad"
}

# ---------- doctor：環境體檢（唯讀零額度；--probe 才花錢實測寫入） ----------
# 純檢查函式（吃路徑、印問題、無副作用）→ self-test 可覆蓋；
# run_doctor 只負責把它們串起來印 ✅/❌。
DOCTOR_FAIL=0
d_ok()   { printf '  ✅ %s\n' "$1"; }
d_bad()  { printf '  ❌ %s\n     修法：%s\n' "$1" "$2"; DOCTOR_FAIL=$((DOCTOR_FAIL+1)); }
d_info() { printf '  ℹ️  %s\n' "$1"; }

doctor_tree_check() { # <repo> → 每行印一個缺少的 .ai/ 路徑；全齊則無輸出
  local p
  for p in CONTRACT.md schedule.yml tasks/backlog.yaml tasks/doing.yaml \
           tasks/done.yaml state/checkpoint.json rubrics agents receipts reports; do
    [ -e "$1/.ai/$p" ] || echo ".ai/$p"
  done
}

doctor_residue_check() { # <file> → 印未填完的 {{PLACEHOLDER}}（去重）；乾淨則無輸出
  grep -o '{{[A-Za-z_]*}}' "$1" 2>/dev/null | sort -u
}

doctor_perm_drift() { # <allow|deny> <模板 settings.json> <目標> → 印目標缺少的條目
  # 跳過 {{…}} 佔位條目（那是 /ai-init 訪談要換掉的，不算漂移）
  local sec="$1" tpl="$2" tgt="$3" e
  if [ "$HAVE_JQ" = 1 ]; then
    jq -r ".permissions.${sec}[]" "$tpl" 2>/dev/null | grep -v '{{' | while IFS= read -r e; do
      jq -e --arg s "$sec" --arg e "$e" '.permissions[$s] | index($e)' "$tgt" >/dev/null 2>&1 || echo "$e"
    done
  else
    sed -n "/\"$sec\"/,/\]/p" "$tpl" | grep -oE '"[^"]+\([^"]*\)"' | tr -d '"' | grep -v '{{' \
    | while IFS= read -r e; do
        grep -qF "\"$e\"" "$tgt" || echo "$e"
      done
  fi
}

run_probe() { # headless 寫入實測：不走 /work（不吃任務、不動 agent 狀態）
  echo "probe：headless 寫入實測（spawn 一次 claude -p，花少量額度，3 分鐘 watchdog）"
  local pf="$REPO/.ai/supervisor/probe.txt" pout ppid pdeadline
  pout="$SUP_DIR/probe-out.json"
  rm -f "$pf"
  ( cd "$REPO"; exec claude -p "用 Write 工具建立檔案 .ai/supervisor/probe.txt，內容只有兩個字元：OK。完成後印出一行：AIOS_PROBE: DONE" \
      --output-format json --model "$MODEL" $PERM_FLAG ) >"$pout" 2>&1 &
  ppid=$!
  pdeadline=$(( $(date +%s) + 180 ))
  while kill -0 "$ppid" 2>/dev/null; do
    if [ "$(date +%s)" -gt "$pdeadline" ]; then
      kill -TERM "$ppid" 2>/dev/null; sleep 2; kill -KILL "$ppid" 2>/dev/null; break
    fi
    sleep 3
  done
  wait "$ppid" 2>/dev/null || true
  if [ -f "$pf" ] && grep -q OK "$pf"; then
    rm -f "$pf"   # supervisor 是 bash，不受 agent 的 rm deny 約束
    d_ok "headless Write 放行——probe 檔已寫入並清除"
  else
    d_bad "headless 寫入失敗（probe 檔沒出現）" \
      "AI-RUNTIME 已知限制 4：(a) 目標 repo settings.local.json 要有 Edit(**)/Write(**) allow (b) 確認不是從 Claude session 巢狀執行 (c) 信任的 repo 才考慮 --yolo。原始輸出：$pout"
  fi
}

run_doctor() {
  echo "doctor：環境體檢 repo=$REPO"
  local miss res tgt tpl drift r k lpid
  # 巢狀 session：權限測試結果會失真（AI-RUNTIME 已知限制 4 的成因）
  if [ -n "${CLAUDECODE:-}${CLAUDE_CODE_ENTRYPOINT:-}" ]; then
    printf '  ⚠️  你在 Claude session 裡面——權限相關結果不代表真實 headless 行為，\n'
    printf '     請在一般終端機重跑本體檢（AI-RUNTIME 已知限制 4）\n'
  fi
  # 工具鏈
  if command -v claude >/dev/null 2>&1; then
    d_ok "claude CLI：$(claude --version 2>/dev/null | head -1)"
  else
    d_bad "找不到 claude CLI" "安裝 Claude Code 並確認在 PATH"
  fi
  if [ "$HAVE_JQ" = 1 ]; then d_info "jq：有（JSON 解析走 jq）"
  else d_info "jq：無（退回 grep/sed 保守萃取——可用，裝了更穩）"; fi
  # .ai/ 樹
  miss=$(doctor_tree_check "$REPO")
  if [ -z "$miss" ]; then d_ok ".ai/ 目錄結構完整"
  else d_bad ".ai/ 結構缺：$(printf '%s' "$miss" | tr '\n' ' ')" "重跑 /ai-init，或從 templates/ai/ 補檔"; fi
  # CONTRACT 訪談殘留
  if [ -f "$REPO/.ai/CONTRACT.md" ]; then
    res=$(doctor_residue_check "$REPO/.ai/CONTRACT.md")
    if [ -z "$res" ]; then d_ok "CONTRACT.md 訪談已填完（無 {{ 殘留）"
    else d_bad "CONTRACT.md 有未填 placeholder：$(printf '%s' "$res" | tr '\n' ' ')" "/ai-init 的訪談沒做完——補填或重跑"; fi
  fi
  # settings.local.json
  tgt="$REPO/.claude/settings.local.json"
  if [ ! -f "$tgt" ]; then
    d_bad "缺 .claude/settings.local.json" "重跑 /ai-init（它會從 templates/ai/settings.local.json 併入）"
  else
    if [ "$HAVE_JQ" = 1 ] && ! jq -e . "$tgt" >/dev/null 2>&1; then
      d_bad "settings.local.json 不是合法 JSON" "手動修復或從模板重併"
    fi
    res=$(doctor_residue_check "$tgt")
    if [ -n "$res" ]; then
      d_bad "settings.local.json 有未填 placeholder：$(printf '%s' "$res" | tr '\n' ' ')" "把 {{TEST_COMMAND}}/{{BUILD_COMMAND}} 換成真實指令"
    fi
    if grep -qF '"Edit(**)"' "$tgt" && grep -qF '"Write(**)"' "$tgt"; then
      d_ok "Edit(**)/Write(**) allow 條目在（headless 寫檔的必要條件）"
    else
      d_bad "settings.local.json 缺 Edit(**) 或 Write(**) allow" "headless -p 下 acceptEdits 不足以放行——補上這兩條"
    fi
    tpl="$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)/templates/ai/settings.local.json"
    if [ -f "$tpl" ]; then
      drift=$(doctor_perm_drift deny "$tpl" "$tgt")
      if [ -z "$drift" ]; then d_ok "deny 清單與模板一致（硬防線沒有漂移）"
      else d_bad "deny 清單缺模板條目：$(printf '%s' "$drift" | tr '\n' ' ')" "手動補進該 repo 的 settings.local.json（deny 是硬防線）"; fi
      drift=$(doctor_perm_drift allow "$tpl" "$tgt")
      if [ -z "$drift" ]; then d_ok "allow 清單涵蓋模板條目"
      else d_bad "allow 清單缺模板條目：$(printf '%s' "$drift" | tr '\n' ' ')" "手動補進該 repo 的 settings.local.json——缺的指令 headless 下會被無聲拒絕"; fi
    else
      d_info "找不到模板 settings.local.json（supervisor 被單獨複製出去？）——跳過 drift 檢查"
    fi
  fi
  # 目標 repo 的 skills
  miss=""
  for k in work review ai-task ai-answer ai-wrap; do
    [ -f "$REPO/.claude/skills/$k/SKILL.md" ] || miss="$miss $k"
  done
  if [ -z "$miss" ]; then d_ok "目標 repo skills 齊全（work/review/ai-task/ai-answer/ai-wrap）"
  else d_bad "目標 repo 缺 skills：$miss" "從 templates/skills/ 複製到 {repo}/.claude/skills/（協定漂移的常見根因）"; fi
  # 狀態檔結構（doctor 是人看的，這裡做硬判定）
  r=$(lint_checkpoint "$REPO/.ai/state/checkpoint.json")
  if [ "$r" = ok ]; then d_ok "checkpoint.json 結構"
  else d_bad "checkpoint.json：$r" "下一輪 /work 會自癒；急的話依 AI-RUNTIME schema 手動重置"; fi
  for k in backlog doing done; do
    r=$(lint_tasks "$REPO/.ai/tasks/$k.yaml" "$k")
    if [ "$r" = ok ]; then d_ok "${k}.yaml 結構"
    else d_bad "${k}.yaml：$r" "下一輪 /work 會自癒（done.yaml 救不回是改名保留，不會清空）"; fi
  done
  # 旗標
  if [ -f "$REPO/.ai/STOP" ]; then
    d_info "STOP 旗標存在：$(head -1 "$REPO/.ai/STOP" 2>/dev/null)——supervisor 不會啟動"
  fi
  if [ -f "$REPO/.ai/PAUSED" ]; then
    if grep -q '^## 人類回覆' "$REPO/.ai/PAUSED" 2>/dev/null; then
      d_info "PAUSED 已有人類回覆——下一輪 /work 會消化並清旗"
    else
      d_info "PAUSED 等待回答中：$(head -1 "$REPO/.ai/PAUSED" 2>/dev/null)（panel 或 /ai-answer 回覆）"
    fi
  fi
  if [ -f "$SUP_DIR/lock" ]; then
    lpid=$(cat "$SUP_DIR/lock" 2>/dev/null || echo "")
    if [ -n "$lpid" ] && kill -0 "$lpid" 2>/dev/null; then
      d_info "supervisor 正在跑（pid $lpid）"
    else
      d_info "殘留 lock（pid ${lpid:-?} 已死）——下次啟動會自動清掉"
    fi
  fi
  if [ -f "$SUP_DIR/events.jsonl" ] && [ "$(stat -f %z "$SUP_DIR/events.jsonl" 2>/dev/null || echo 0)" -gt 1048576 ]; then
    d_info "events.jsonl 超過 1MB——非稽核檔，直接 truncate 或刪掉即可"
  fi
  [ "$YOLO" = 1 ] && d_info "注意：--yolo 會跳過上面驗證的所有權限防線"
  # probe（可選，花錢）
  [ "$PROBE" = 1 ] && run_probe
  echo ""
  if [ "$DOCTOR_FAIL" = 0 ]; then echo "doctor：全部通過"
  else echo "doctor：$DOCTOR_FAIL 項失敗（修法見各 ❌）"; fi
  [ "$DOCTOR_FAIL" = 0 ]
}

# ---------- self-test：零額度驗證分類器 ----------
run_self_test() {
  local dir pass=0 fail=0
  dir=$(mktemp -d)
  t() { # t <名稱> <期望類別> <exit_code> <內容>
    printf '%s' "$4" > "$dir/f"
    local got; got=$(classify "$3" "$dir/f")
    if [ "$got" = "$2" ]; then pass=$((pass+1)); echo "  ok  $1 -> $got"
    else fail=$((fail+1)); echo "  FAIL $1 -> got=$got want=$2"; fi
  }
  echo "supervisor self-test（分類器 + 睡眠計算）"
  t "正常完成"       productive  0 '{"result":"...done\nAIOS_STATUS: DONE_TASK task=T-001 score=85 receipt=x","total_cost_usd":0.12}'
  t "部分完成"       productive  0 '{"result":"AIOS_STATUS: TASK_PARTIAL task=T-002 score=na receipt=y"}'
  t "佇列空"         queue_empty 0 '{"result":"AIOS_STATUS: QUEUE_EMPTY task=none score=na receipt=none"}'
  t "暫停等人"       paused      0 '{"result":"AIOS_STATUS: PAUSED task=T-003 score=na receipt=none"}'
  t "手動停止"       stopped     0 '{"result":"AIOS_STATUS: STOPPED task=none score=na receipt=none"}'
  t "額度用盡"       rate_limit  1 "You've hit your usage limit · resets 8pm (Asia/Taipei)"
  t "session limit"  rate_limit  1 "You've hit your session limit · resets 10am"
  t "session limit 帶分鐘" rate_limit 1 "You've hit your session limit · resets 6:50am (Asia/Taipei)
/upgrade or /usage-credits to finish what you're working on."
  t "limit+classifier 混合" rate_limit 1 "Error: claude-opus-4-8[1m] is temporarily unavailable, so auto mode cannot determine the safety of Write right now.
You've hit your session limit · resets 6:50am (Asia/Taipei)"
  t "limit reached+epoch" rate_limit 1 'Claude AI usage limit reached|1752555600'
  t "API rate_limit_error" rate_limit 1 '{"type":"error","error":{"type":"rate_limit_error","message":"Number of request tokens has exceeded your per-minute rate limit"}}'
  t "HTTP 429 裸碼"       rate_limit  1 'API Error: 429 {"type":"error"}'
  t "resets 帶 at"        rate_limit  1 'Your limit will replenish · resets at 8pm (Asia/Taipei)'
  t "weekly limit 帶日期" rate_limit  1 "You've hit your weekly limit · resets Jul 17 at 12pm (Asia/Taipei)"
  t "網路斷線"       network     1 'Error: fetch failed ECONNRESET'
  t "伺服器過載"     network     1 'API Error: 529 overloaded_error'
  t "classifier 暫時不可用" network 1 'Error: claude-opus-4-8[1m] is temporarily unavailable, so auto mode cannot determine the safety of Write right now. Wait briefly and then try this action again.'
  t "watchdog 殺掉"  killed      137 ''
  t "崩潰"           crash       1 'segfault or whatever'
  t "is_error"       crash       0 '{"is_error": true, "result":"boom"}'
  t "協定漂移"       no_status   0 '{"result":"我做完了但忘記印狀態行"}'
  echo "睡眠計算（AIOS_FAKE_SLEEP=1）："
  ts() { # ts <名稱> <輸出需含> <訊息> —— 斷言睡眠走對分支且真的睡了
    local got; got=$(AIOS_FAKE_SLEEP=1 RL_FALLBACK_MIN=30 sleep_until_reset "$3" 2>&1)
    if printf '%s' "$got" | grep -q "$2" && printf '%s' "$got" | grep -q 'FAKE_SLEEP'; then
      pass=$((pass+1)); echo "  ok  $1"
    else fail=$((fail+1)); echo "  FAIL $1 -> $got"; fi
  }
  ts "resets 8pm→睡到 reset"       '睡到' 'resets 8pm blah'
  ts "resets 6:50am→睡到 reset"    '睡到' 'resets 6:50am (Asia/Taipei)'
  ts "resets at 8pm→睡到 reset"    '睡到' "You've hit your usage limit · resets at 8pm (Asia/Taipei)"
  ts "epoch 已過→floor 120s"       'FAKE_SLEEP 120s' 'Claude AI usage limit reached|100000000'
  ts "無法解析→fallback 30 分鐘"   '無法解析' '無法解析的訊息'
  echo "quota 文字解析（parse_usage_pct）："
  usage_txt='Current session: 6% used · resets Jul 9 at 11:30pm (Asia/Taipei)
Current week (all models): 44% used · resets Jul 15 at 12pm (Asia/Taipei)
Current week (Fable): 46% used · resets Jul 15 at 12pm (Asia/Taipei)'
  tq() { # tq <名稱> <label> <期望值>
    local got; got=$(parse_usage_pct "$usage_txt" "$2")
    if [ "$got" = "$3" ]; then pass=$((pass+1)); echo "  ok  $1 -> $got"
    else fail=$((fail+1)); echo "  FAIL $1 -> got=$got want=$3"; fi
  }
  tq "5h 用量"  session 6
  tq "7d 用量"  week    44
  echo "quota 決策（quota_decide sess week wait stop）："
  td() { # td <名稱> <sess> <week> <wait> <stop> <期望>
    local got; got=$(quota_decide "$2" "$3" "$4" "$5")
    if [ "$got" = "$6" ]; then pass=$((pass+1)); echo "  ok  $1 -> $got"
    else fail=$((fail+1)); echo "  FAIL $1 -> got=$got want=$6"; fi
  }
  td "低用量放行"        30 40 60 80 go
  td "5h 軟門檻等待"     65 40 60 80 wait
  td "5h 超硬門檻仍等待" 85 40 60 80 wait
  td "5h 100% 仍等待"    100 40 60 80 wait
  td "7d 硬門檻停止"     30 85 60 80 stop
  td "7d 不觸發軟等待"   30 70 60 80 go
  td "軟門檻停用(0)"     70 40 0  80 go
  td "查無用量放行"      "" "" 60 80 go
  echo "狀態檔 lint（lint_checkpoint / lint_tasks）："
  tl() { # tl <名稱> <want: ok|bad> <lint 輸出>
    local verdict=bad; [ "$3" = ok ] && verdict=ok
    if [ "$verdict" = "$2" ]; then pass=$((pass+1)); echo "  ok  $1"
    else fail=$((fail+1)); echo "  FAIL $1 -> want=$2 got=$3"; fi
  }
  printf '{"version":1,"phase":"executing","task_step":"3"}' > "$dir/ck.json"
  tl "checkpoint 合法"        ok  "$(lint_checkpoint "$dir/ck.json")"
  printf '{"version":1,"pha' > "$dir/ck.json"
  tl "checkpoint 截斷 JSON"   bad "$(lint_checkpoint "$dir/ck.json")"
  printf '[]' > "$dir/ck.json"
  tl "checkpoint 不是 object" bad "$(lint_checkpoint "$dir/ck.json")"
  printf 'version: 1\ntasks:\n  - id: T-001\n    title: x\n' > "$dir/t.yaml"
  tl "backlog 合法"           ok  "$(lint_tasks "$dir/t.yaml" backlog)"
  printf 'version: 1\ntasks: []\n' > "$dir/t.yaml"
  tl "空佇列合法"             ok  "$(lint_tasks "$dir/t.yaml" backlog)"
  printf 'version: 1\ntasks:\n\t- id: T-001\n' > "$dir/t.yaml"
  tl "tab 縮排"               bad "$(lint_tasks "$dir/t.yaml" backlog)"
  printf 'version: 1\nitems:\n' > "$dir/t.yaml"
  tl "缺 tasks: key"          bad "$(lint_tasks "$dir/t.yaml" backlog)"
  printf 'version: 1\ntasks:\n  - id: T-001\n  - id: T-002\n' > "$dir/t.yaml"
  tl "doing 兩筆"             bad "$(lint_tasks "$dir/t.yaml" doing)"
  printf 'version: 1\ntasks:\n  - id: X-01\n' > "$dir/t.yaml"
  tl "id 非 T-NNN"            bad "$(lint_tasks "$dir/t.yaml" done)"
  echo "doctor 純檢查（tree / placeholder 殘留 / deny-drift）："
  mkdir -p "$dir/repo/.ai/tasks" "$dir/repo/.ai/state" "$dir/repo/.ai/rubrics" \
           "$dir/repo/.ai/agents" "$dir/repo/.ai/receipts" "$dir/repo/.ai/reports"
  : > "$dir/repo/.ai/CONTRACT.md"; : > "$dir/repo/.ai/schedule.yml"
  for k in backlog doing done; do : > "$dir/repo/.ai/tasks/$k.yaml"; done
  : > "$dir/repo/.ai/state/checkpoint.json"
  tl "tree 完整"     ok  "$([ -z "$(doctor_tree_check "$dir/repo")" ] && echo ok || echo bad)"
  rm "$dir/repo/.ai/tasks/doing.yaml"
  tl "tree 缺 doing" bad "$([ -z "$(doctor_tree_check "$dir/repo")" ] && echo ok || echo bad)"
  printf 'mission: {{MISSION}}\n' > "$dir/repo/.ai/CONTRACT.md"
  tl "CONTRACT {{ 殘留" bad "$([ -z "$(doctor_residue_check "$dir/repo/.ai/CONTRACT.md")" ] && echo ok || echo bad)"
  printf 'mission: real text\n' > "$dir/repo/.ai/CONTRACT.md"
  tl "CONTRACT 已填完"  ok  "$([ -z "$(doctor_residue_check "$dir/repo/.ai/CONTRACT.md")" ] && echo ok || echo bad)"
  printf '{\n  "permissions": {\n    "allow": [\n      "Edit(**)",\n      "Bash(date:*)",\n      "Bash({{TEST_COMMAND}}:*)"\n    ],\n    "deny": [\n      "Bash(git push:*)",\n      "Bash(rm:*)"\n    ]\n  }\n}\n' > "$dir/tpl.json"
  printf '{\n  "permissions": {\n    "allow": [\n      "Edit(**)"\n    ],\n    "deny": [\n      "Bash(git push:*)"\n    ]\n  }\n}\n' > "$dir/tgt.json"
  tl "deny-drift 抓到缺條目"  ok "$([ "$(doctor_perm_drift deny "$dir/tpl.json" "$dir/tgt.json")" = 'Bash(rm:*)' ] && echo ok || echo bad)"
  tl "allow-drift 跳過 {{ 佔位" ok "$([ "$(doctor_perm_drift allow "$dir/tpl.json" "$dir/tgt.json")" = 'Bash(date:*)' ] && echo ok || echo bad)"
  cp "$dir/tpl.json" "$dir/tgt.json"
  tl "drift 無漂移"           ok "$([ -z "$(doctor_perm_drift deny "$dir/tpl.json" "$dir/tgt.json")$(doctor_perm_drift allow "$dir/tpl.json" "$dir/tgt.json")" ] && echo ok || echo bad)"
  echo "emit_event（JSON 逃逸與欄位消毒）："
  SUP_DIR="$dir"; iter=7; class=productive; status_tok=DONE_TASK; task_tok='T-001"x'; cost=0.12
  emit_event iteration "detail 帶 \"引號\"、反斜線 \\ 與
換行還有	tab"
  emit_event quota_wait "5h=65% >= 60%"
  SUP_DIR=""
  ev_ok=1
  [ "$(wc -l < "$dir/events.jsonl" | tr -d ' ')" = 2 ] || ev_ok=0
  if [ "$HAVE_JQ" = 1 ]; then
    jq -e . "$dir/events.jsonl" >/dev/null 2>&1 || ev_ok=0
    [ "$(jq -r 'select(.event=="iteration").task' "$dir/events.jsonl")" = "T-001x" ] || ev_ok=0
    [ "$(jq -r 'select(.event=="iteration").cost_usd' "$dir/events.jsonl")" = "0.12" ] || ev_ok=0
  else
    grep -q '"event":"iteration"' "$dir/events.jsonl" || ev_ok=0
    grep -q '"task":"T-001x"' "$dir/events.jsonl" || ev_ok=0
  fi
  tl "兩行事件、值已消毒、合法 JSON" ok "$([ "$ev_ok" = 1 ] && echo ok || echo bad)"
  rm -rf "$dir"
  echo "self-test: pass=$pass fail=$fail"
  [ "$fail" = 0 ]
}

if [ "$SELF_TEST" = 1 ]; then
  SUP_DIR="" SCHED="" RL_FALLBACK_MIN=30
  run_self_test; exit $?
fi

# ---------- 前置檢查 ----------
[ -n "$REPO" ] || { echo "--repo 必填（或用 --self-test）" >&2; exit 64; }
[ -d "$REPO/.ai" ] || { echo "$REPO/.ai 不存在——先跑 /ai-init" >&2; exit 66; }
command -v claude >/dev/null 2>&1 || { echo "找不到 claude CLI" >&2; exit 69; }

SCHED="$REPO/.ai/schedule.yml"
SUP_DIR="$REPO/.ai/supervisor"; mkdir -p "$SUP_DIR"

MAX_ITER="${MAX_ITER:-$(sched_get max_iterations_per_run 10)}"
MAX_FAIL="${MAX_FAIL:-$(sched_get max_consecutive_failures 3)}"
TIMEOUT_MIN=$(sched_get iteration_timeout_minutes 30)
SLEEP_BETWEEN=$(sched_get sleep_between_iterations_seconds 20)
RL_FALLBACK_MIN=$(sched_get rate_limit_fallback_sleep_minutes 30)
NET_BASE=$(sched_get network_backoff_base_seconds 30)
NET_MAX=$(sched_get network_backoff_max_seconds 900)
MAX_COST=$(sched_get max_cost_per_run_usd 5)
MODEL="${MODEL:-$(sched_get claude_model sonnet)}"
SCHED_FLAGS=$(sched_get extra_claude_flags "")
REVIEW="${REVIEW:-$(sched_get review_after_task false)}"
[ "$ONCE" = 1 ] && MAX_ITER=1

PERM_FLAG="--permission-mode acceptEdits"
[ "$YOLO" = 1 ] && PERM_FLAG="--dangerously-skip-permissions"

if [ "$DOCTOR" = 1 ]; then
  run_doctor; exit $?
fi

# lock（單 repo 單 supervisor；殘留 lock 以 pid 存活判定）
LOCK="$SUP_DIR/lock"
if [ -f "$LOCK" ]; then
  oldpid=$(cat "$LOCK" 2>/dev/null || echo "")
  if [ -n "$oldpid" ] && kill -0 "$oldpid" 2>/dev/null; then
    echo "另一個 supervisor（pid ${oldpid}）正在跑這個 repo" >&2; exit 75
  fi
  log "清掉殘留 lock（pid ${oldpid:-?} 已不存在）"
fi
echo $$ > "$LOCK"
trap 'rm -f "$LOCK"' EXIT

if [ "$DRY_RUN" = 1 ]; then
  echo "dry-run：repo=$REPO model=$MODEL max_iter=$MAX_ITER max_fail=$MAX_FAIL"
  echo "  timeout=${TIMEOUT_MIN}m sleep=${SLEEP_BETWEEN}s max_cost=\$${MAX_COST} perm=$PERM_FLAG"
  echo "  cmd: (cd $REPO && claude -p \"/work\" --output-format json --model $MODEL $PERM_FLAG $SCHED_FLAGS $EXTRA_FLAGS)"
  exit 0
fi

# ---------- 主迴圈 ----------
# 啟動前狀態檔結構檢查（只警告不阻擋——壞檔自癒本身就是 /work 的恢復路徑）
lint_bad=$(lint_state)
[ "$lint_bad" = 0 ] || log "state lint：${lint_bad} 個狀態檔結構異常（見上），預期下一輪 /work 依協定自癒"

iter=0 consecutive_failures=0 net_retries=0 nostatus_count=0 total_cost=0 rl_rounds=0
class="" status_tok="" task_tok="" cost=0 END_REASON=""
# run_end 掛在 EXIT trap：不管從哪個出口離開都會記一筆（原因在 END_REASON）
trap 'rm -f "$LOCK"; emit_event run_end "reason=${END_REASON:-exit} total_cost=${total_cost:-0}"' EXIT
log "supervisor 啟動：repo=$REPO model=$MODEL max_iter=$MAX_ITER perm=$PERM_FLAG"
emit_event run_start "model=$MODEL max_iter=$MAX_ITER"

while [ "$iter" -lt "$MAX_ITER" ]; do
  iter=$((iter+1))
  class="" status_tok="" task_tok="" cost=0   # 不讓事件帶到上一輪的殘值

  if [ -f "$REPO/.ai/STOP" ]; then log "發現 .ai/STOP，結束"; END_REASON=stop_flag; exit 0; fi
  if [ -f "$REPO/.ai/PAUSED" ] && ! grep -q '^## 人類回覆' "$REPO/.ai/PAUSED" 2>/dev/null; then
    log "等待人類：$(head -3 "$REPO/.ai/PAUSED" 2>/dev/null)"
    if [ "$WAIT_ON_PAUSE" = 1 ]; then do_sleep 300; iter=$((iter-1)); continue; fi
    END_REASON=paused; exit 2
  fi
  if [ -f "$REPO/.ai/PAUSED" ]; then
    log "PAUSED 已有人類回覆，跑一輪 /work 讓它消化並清旗"
  fi
  quota_check || exit 0

  ckpt="$REPO/.ai/state/checkpoint.json"
  mtime_before=$(stat -f %m "$ckpt" 2>/dev/null || echo 0)

  out="$SUP_DIR/out.json"; err="$SUP_DIR/err.log"; : > "$out"; : > "$err"
  log "iteration $iter/$MAX_ITER 開始"

  # watchdog：背景執行 + 輪詢（macOS 無 coreutils timeout）。
  # exec 讓 claude 取代子 shell，kill $cpid 才殺得到 claude 本體而非 wrapper。
  ( cd "$REPO"; exec claude -p "/work" --output-format json --model "$MODEL" $PERM_FLAG $SCHED_FLAGS $EXTRA_FLAGS ) >"$out" 2>"$err" &
  cpid=$!
  deadline=$(( $(date +%s) + TIMEOUT_MIN * 60 ))
  while kill -0 "$cpid" 2>/dev/null; do
    if [ "$(date +%s)" -gt "$deadline" ]; then
      log "watchdog：超過 ${TIMEOUT_MIN} 分鐘，砍掉 pid $cpid"
      emit_event watchdog_kill "超過 ${TIMEOUT_MIN} 分鐘"
      kill -TERM "$cpid" 2>/dev/null; sleep 5; kill -KILL "$cpid" 2>/dev/null
      break
    fi
    sleep 5
  done
  wait "$cpid" 2>/dev/null; ec=$?

  combined="$SUP_DIR/combined.txt"; cat "$out" "$err" > "$combined" 2>/dev/null
  cost=$(extract_cost < "$out")
  total_cost=$(awk -v a="$total_cost" -v b="$cost" 'BEGIN{printf "%.4f", a+b}')
  class=$(classify "$ec" "$combined")
  status_tok=$(grep -oE 'AIOS_STATUS: [A-Z_]+' "$combined" | tail -1 | awk '{print $2}')
  task_tok=$(grep -oE 'task=[^ ]+' "$combined" | tail -1 | cut -d= -f2)
  # 「連續」網路失敗計數：只要這輪不是網路錯誤就歸零
  [ "$class" != "network" ] && net_retries=0
  [ "$class" != "rate_limit" ] && rl_rounds=0
  log "iteration ${iter}：class=$class exit=$ec cost=\$${cost} total=\$${total_cost}"
  printf '{"iteration":%s,"last_status":"%s","consecutive_failures":%s,"total_cost_usd":%s,"at":"%s"}\n' \
    "$iter" "$class" "$consecutive_failures" "$total_cost" "$(date '+%Y-%m-%dT%H:%M:%S')" > "$SUP_DIR/last_run.json"
  emit_event iteration

  # 成本熔斷
  if awk -v t="$total_cost" -v m="$MAX_COST" 'BEGIN{exit !(t>m)}'; then
    log "成本熔斷：\$${total_cost} > \$${MAX_COST}，停止"
    END_REASON=cost_breaker; emit_event cost_breaker "total=${total_cost} > max=${MAX_COST}"
    exit 3
  fi

  case "$class" in
    stopped)     log "agent 回報 STOPPED"; END_REASON=stopped; exit 0 ;;
    paused)      log "agent 需要人類：$(head -3 "$REPO/.ai/PAUSED" 2>/dev/null || echo '(見 receipts)')"
                 [ "$WAIT_ON_PAUSE" = 1 ] && { do_sleep 300; continue; } || { END_REASON=paused; exit 2; } ;;
    queue_empty) log "佇列空，正常收工"; END_REASON=queue_empty; exit 0 ;;
    productive)
      # 交叉驗證：agent 說有進展，checkpoint 就該被動過
      mtime_after=$(stat -f %m "$ckpt" 2>/dev/null || echo 0)
      if [ "$mtime_after" = "$mtime_before" ]; then
        consecutive_failures=$((consecutive_failures+1))
        log "警告：回報 productive 但 checkpoint 未更新（協定違規），計失敗 $consecutive_failures/$MAX_FAIL"
      else
        consecutive_failures=0 net_retries=0
        # 多 agent 審查輪：DONE_TASK 之後開全新 session 獨立審查（可選）
        if [ "$REVIEW" = "true" ] && [ "$status_tok" = "DONE_TASK" ]; then
          log "review 輪：開 fresh session 審查上一個任務"
          rout="$SUP_DIR/review-out.json"
          ( cd "$REPO"; exec claude -p "/review" --output-format json --model "$MODEL" $PERM_FLAG $SCHED_FLAGS $EXTRA_FLAGS ) >"$rout" 2>>"$err"
          rcost=$(extract_cost < "$rout")
          total_cost=$(awk -v a="$total_cost" -v b="$rcost" 'BEGIN{printf "%.4f", a+b}')
          rline=$(extract_result < "$rout" | grep -oE 'AIOS_REVIEW: (PASS|FAIL)[^"]*' | tail -1)
          log "review 結果：${rline:-（無 AIOS_REVIEW 行——檢查 /review skill 是否已安裝）} cost=\$${rcost}"
          # FAIL 時 reviewer 已把修正任務排進 backlog，下一輪 /work 自然接手
        fi
      fi
      # checkpoint 結構驗證（只在有 jq 時做硬判定——無 jq 的淺檢查可能誤判，
      # 誤判不可以計失敗殺迴圈；壞 JSON 本身下一輪 /work 會自癒）
      if [ "$HAVE_JQ" = 1 ]; then
        ck_reason=$(lint_checkpoint "$ckpt")
        if [ "$ck_reason" != ok ]; then
          consecutive_failures=$((consecutive_failures+1))
          log "警告：回報 productive 但 checkpoint 結構壞掉（$ck_reason），計失敗 $consecutive_failures/$MAX_FAIL"
        fi
      fi
      do_sleep "$SLEEP_BETWEEN" ;;
    rate_limit)
      rl_rounds=$((rl_rounds+1))
      if [ "$rl_rounds" -gt 8 ]; then log "連續 rate-limit 超過 8 輪（約兩天），放棄——檢查帳號額度"; END_REASON=rate_limit_giveup; exit 1; fi
      emit_event rate_limit_sleep "round ${rl_rounds}"
      sleep_until_reset "$(cat "$combined")"; iter=$((iter-1)) ;;   # 不計輪、不計失敗，但有自己的上限
    network)
      net_retries=$((net_retries+1))
      if [ "$net_retries" -gt 6 ]; then log "網路重試超過 6 次，放棄"; END_REASON=network_giveup; exit 1; fi
      backoff=$(( NET_BASE * (1 << (net_retries-1)) ))
      [ "$backoff" -gt "$NET_MAX" ] && backoff=$NET_MAX
      log "網路錯誤，backoff ${backoff}s（第 $net_retries 次）"
      do_sleep "$backoff"; iter=$((iter-1)) ;;
    killed|crash)
      consecutive_failures=$((consecutive_failures+1))
      log "失敗（${class}）$consecutive_failures/${MAX_FAIL}，60s 後重試"
      do_sleep 60 ;;
    no_status)
      nostatus_count=$((nostatus_count+1))
      log "協定漂移：exit 0 但無 AIOS_STATUS（$nostatus_count/3）"
      if [ "$nostatus_count" -ge 3 ]; then log "協定漂移過多，停止——檢查 /work skill 是否還在目標 repo"; END_REASON=protocol_drift; exit 1; fi
      do_sleep "$SLEEP_BETWEEN" ;;
  esac

  if [ "$consecutive_failures" -ge "$MAX_FAIL" ]; then
    log "連續失敗達 $MAX_FAIL 次，停止。詳見 $SUP_DIR/{err.log,out.json}"
    END_REASON=failure_ceiling; exit 1
  fi
done

log "達 max_iterations（${MAX_ITER}），收工。總成本 \$${total_cost}"
END_REASON=max_iterations
exit 0
