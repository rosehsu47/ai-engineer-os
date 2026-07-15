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
#
# 安全閥：單 repo 單 supervisor（lock）、max_iterations、連續失敗上限、
# 每輪 watchdog timeout、run 累計成本熔斷（max_cost_per_run_usd）、
# .ai/STOP 隨時手動煞車。macOS bash 3.2 相容。
set -u

# ---------- 參數與預設 ----------
REPO="" ONCE=0 YOLO=0 WAIT_ON_PAUSE=0 DRY_RUN=0 VERBOSE=0 SELF_TEST=0 REVIEW=""
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
  if printf '%s' "$all" | grep -qiE 'hit your [a-z ]*limit|(usage|session|weekly) limit|rate.?limit|resets [0-9]{1,2}(:[0-9]{2})?[[:space:]]?(am|pm)'; then
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

# ---------- rate-limit 睡眠（解析 "resets 8pm" 失敗則固定 fallback） ----------
sleep_until_reset() { # $1 = 全部輸出文字
  local ts hour min ampm target now cap
  ts=$(printf '%s' "$1" | grep -oiE 'resets [0-9]{1,2}(:[0-9]{2})?[[:space:]]?(am|pm)' | head -1)
  hour=$(printf '%s' "$ts" | grep -oE '[0-9]{1,2}' | head -1)
  min=$(printf '%s' "$ts" | grep -oE ':[0-9]{2}' | head -1 | tr -d ':')
  ampm=$(printf '%s' "$ts" | grep -oiE '(am|pm)' | tail -1 | tr 'A-Z' 'a-z')
  now=$(date +%s)
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
        return 1 ;;
      wait)
        if [ "$waited_min" -ge 1440 ]; then
          log "quota 軟門檻等了 24h 仍未降（5h=${sess}%——個人使用持續占用？），寫 STOP 收工"
          printf 'quota 軟門檻等待逾 24h（%s）\n5h 用量持續 ≥ %s%%\n' \
            "$(date '+%Y-%m-%dT%H:%M:%S')" "$wait_t" > "$REPO/.ai/STOP"
          return 1
        fi
        log "quota 軟門檻：5h 已用 ${sess}%（≥${wait_t}%），不開新任務，${recheck} 分鐘後再查（已等 ${waited_min} 分）"
        do_sleep $((recheck * 60)); waited_min=$((waited_min + recheck))
        [ -f "$REPO/.ai/STOP" ] && { log "等待期間發現 .ai/STOP，結束"; return 1; } ;;
    esac
  done
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
  t "網路斷線"       network     1 'Error: fetch failed ECONNRESET'
  t "伺服器過載"     network     1 'API Error: 529 overloaded_error'
  t "classifier 暫時不可用" network 1 'Error: claude-opus-4-8[1m] is temporarily unavailable, so auto mode cannot determine the safety of Write right now. Wait briefly and then try this action again.'
  t "watchdog 殺掉"  killed      137 ''
  t "崩潰"           crash       1 'segfault or whatever'
  t "is_error"       crash       0 '{"is_error": true, "result":"boom"}'
  t "協定漂移"       no_status   0 '{"result":"我做完了但忘記印狀態行"}'
  echo "睡眠計算（AIOS_FAKE_SLEEP=1）："
  AIOS_FAKE_SLEEP=1 RL_FALLBACK_MIN=30 sleep_until_reset "resets 8pm blah" | sed 's/^/  /'
  AIOS_FAKE_SLEEP=1 RL_FALLBACK_MIN=30 sleep_until_reset "resets 6:50am (Asia/Taipei)" | sed 's/^/  /'
  AIOS_FAKE_SLEEP=1 RL_FALLBACK_MIN=30 sleep_until_reset "無法解析的訊息" | sed 's/^/  /'
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
iter=0 consecutive_failures=0 net_retries=0 nostatus_count=0 total_cost=0 rl_rounds=0
log "supervisor 啟動：repo=$REPO model=$MODEL max_iter=$MAX_ITER perm=$PERM_FLAG"

while [ "$iter" -lt "$MAX_ITER" ]; do
  iter=$((iter+1))

  if [ -f "$REPO/.ai/STOP" ]; then log "發現 .ai/STOP，結束"; exit 0; fi
  if [ -f "$REPO/.ai/PAUSED" ]; then
    log "等待人類：$(head -3 "$REPO/.ai/PAUSED" 2>/dev/null)"
    if [ "$WAIT_ON_PAUSE" = 1 ]; then do_sleep 300; iter=$((iter-1)); continue; fi
    exit 2
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
  # 「連續」網路失敗計數：只要這輪不是網路錯誤就歸零
  [ "$class" != "network" ] && net_retries=0
  [ "$class" != "rate_limit" ] && rl_rounds=0
  log "iteration ${iter}：class=$class exit=$ec cost=\$${cost} total=\$${total_cost}"
  printf '{"iteration":%s,"last_status":"%s","consecutive_failures":%s,"total_cost_usd":%s,"at":"%s"}\n' \
    "$iter" "$class" "$consecutive_failures" "$total_cost" "$(date '+%Y-%m-%dT%H:%M:%S')" > "$SUP_DIR/last_run.json"

  # 成本熔斷
  if awk -v t="$total_cost" -v m="$MAX_COST" 'BEGIN{exit !(t>m)}'; then
    log "成本熔斷：\$${total_cost} > \$${MAX_COST}，停止"; exit 3
  fi

  case "$class" in
    stopped)     log "agent 回報 STOPPED"; exit 0 ;;
    paused)      log "agent 需要人類：$(head -3 "$REPO/.ai/PAUSED" 2>/dev/null || echo '(見 receipts)')"
                 [ "$WAIT_ON_PAUSE" = 1 ] && { do_sleep 300; continue; } || exit 2 ;;
    queue_empty) log "佇列空，正常收工"; exit 0 ;;
    productive)
      # 交叉驗證：agent 說有進展，checkpoint 就該被動過
      mtime_after=$(stat -f %m "$ckpt" 2>/dev/null || echo 0)
      if [ "$mtime_after" = "$mtime_before" ]; then
        consecutive_failures=$((consecutive_failures+1))
        log "警告：回報 productive 但 checkpoint 未更新（協定違規），計失敗 $consecutive_failures/$MAX_FAIL"
      else
        consecutive_failures=0 net_retries=0
        # 多 agent 審查輪：DONE_TASK 之後開全新 session 獨立審查（可選）
        status_tok=$(grep -oE 'AIOS_STATUS: [A-Z_]+' "$combined" | tail -1 | awk '{print $2}')
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
      do_sleep "$SLEEP_BETWEEN" ;;
    rate_limit)
      rl_rounds=$((rl_rounds+1))
      if [ "$rl_rounds" -gt 8 ]; then log "連續 rate-limit 超過 8 輪（約兩天），放棄——檢查帳號額度"; exit 1; fi
      sleep_until_reset "$(cat "$combined")"; iter=$((iter-1)) ;;   # 不計輪、不計失敗，但有自己的上限
    network)
      net_retries=$((net_retries+1))
      if [ "$net_retries" -gt 6 ]; then log "網路重試超過 6 次，放棄"; exit 1; fi
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
      if [ "$nostatus_count" -ge 3 ]; then log "協定漂移過多，停止——檢查 /work skill 是否還在目標 repo"; exit 1; fi
      do_sleep "$SLEEP_BETWEEN" ;;
  esac

  if [ "$consecutive_failures" -ge "$MAX_FAIL" ]; then
    log "連續失敗達 $MAX_FAIL 次，停止。詳見 $SUP_DIR/{err.log,out.json}"; exit 1
  fi
done

log "達 max_iterations（${MAX_ITER}），收工。總成本 \$${total_cost}"
exit 0
