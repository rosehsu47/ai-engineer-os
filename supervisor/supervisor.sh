#!/usr/bin/env bash
# supervisor.sh — AI Engineer OS 的無人監督迴圈
#
# 每輪開一個全新的 `claude -p "/work"` session（狀態都在 .ai/ 檔案裡，
# 不用 --resume），讀取 AIOS_STATUS 與錯誤徵兆做分類與復原。
# 協定見 work-record-tool/AI-RUNTIME.md；錯誤分類表見本檔 classify()。
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
REPO="" ONCE=0 YOLO=0 WAIT_ON_PAUSE=0 DRY_RUN=0 VERBOSE=0 SELF_TEST=0
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
  if printf '%s' "$all" | grep -qiE 'usage limit|rate.?limit|resets [0-9]{1,2}[[:space:]]?(am|pm)'; then
    echo rate_limit; return
  fi
  if printf '%s' "$all" | grep -qiE 'overloaded|529|ECONNRESET|ETIMEDOUT|ENOTFOUND|fetch failed|socket hang'; then
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
  local hour ampm target now cap
  hour=$(printf '%s' "$1" | grep -oiE 'resets [0-9]{1,2}[[:space:]]?(am|pm)' | head -1 | grep -oE '[0-9]{1,2}')
  ampm=$(printf '%s' "$1" | grep -oiE 'resets [0-9]{1,2}[[:space:]]?(am|pm)' | head -1 | grep -oiE '(am|pm)' | tr 'A-Z' 'a-z')
  now=$(date +%s)
  if [ -n "${hour:-}" ] && [ -n "${ampm:-}" ]; then
    target=$(date -j -f '%Y-%m-%d %I%p' "$(date '+%Y-%m-%d') ${hour}${ampm}" +%s 2>/dev/null || echo "")
    if [ -n "$target" ]; then
      [ "$target" -le "$now" ] && target=$((target + 86400))
      target=$((target + 120))                       # reset 後多等 2 分鐘
      cap=$((now + 21600))                           # 上限 6 小時
      [ "$target" -gt "$cap" ] && target=$cap
      log "rate limit: 睡到 $(date -r "$target" '+%H:%M')（resets ${hour}${ampm}）"
      do_sleep $((target - now)); return
    fi
  fi
  log "rate limit: 無法解析 reset 時間，固定睡 ${RL_FALLBACK_MIN} 分鐘"
  do_sleep $((RL_FALLBACK_MIN * 60))
}

do_sleep() { # 可被 AIOS_FAKE_SLEEP=1 假化（self-test 用）
  if [ "${AIOS_FAKE_SLEEP:-0}" = 1 ]; then echo "FAKE_SLEEP ${1}s"; else sleep "$1"; fi
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
  t "網路斷線"       network     1 'Error: fetch failed ECONNRESET'
  t "伺服器過載"     network     1 'API Error: 529 overloaded_error'
  t "watchdog 殺掉"  killed      137 ''
  t "崩潰"           crash       1 'segfault or whatever'
  t "is_error"       crash       0 '{"is_error": true, "result":"boom"}'
  t "協定漂移"       no_status   0 '{"result":"我做完了但忘記印狀態行"}'
  echo "睡眠計算（AIOS_FAKE_SLEEP=1）："
  AIOS_FAKE_SLEEP=1 RL_FALLBACK_MIN=30 sleep_until_reset "resets 8pm blah" | sed 's/^/  /'
  AIOS_FAKE_SLEEP=1 RL_FALLBACK_MIN=30 sleep_until_reset "無法解析的訊息" | sed 's/^/  /'
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
iter=0 consecutive_failures=0 net_retries=0 nostatus_count=0 total_cost=0
log "supervisor 啟動：repo=$REPO model=$MODEL max_iter=$MAX_ITER perm=$PERM_FLAG"

while [ "$iter" -lt "$MAX_ITER" ]; do
  iter=$((iter+1))

  if [ -f "$REPO/.ai/STOP" ]; then log "發現 .ai/STOP，結束"; exit 0; fi
  if [ -f "$REPO/.ai/PAUSED" ]; then
    log "等待人類：$(head -3 "$REPO/.ai/PAUSED" 2>/dev/null)"
    if [ "$WAIT_ON_PAUSE" = 1 ]; then do_sleep 300; iter=$((iter-1)); continue; fi
    exit 2
  fi

  ckpt="$REPO/.ai/state/checkpoint.json"
  mtime_before=$(stat -f %m "$ckpt" 2>/dev/null || echo 0)

  out="$SUP_DIR/out.json"; err="$SUP_DIR/err.log"; : > "$out"; : > "$err"
  log "iteration $iter/$MAX_ITER 開始"

  # watchdog：背景執行 + 輪詢（macOS 無 coreutils timeout）
  ( cd "$REPO" && claude -p "/work" --output-format json --model "$MODEL" $PERM_FLAG $SCHED_FLAGS $EXTRA_FLAGS ) >"$out" 2>"$err" &
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
      fi
      do_sleep "$SLEEP_BETWEEN" ;;
    rate_limit)  sleep_until_reset "$(cat "$combined")"; iter=$((iter-1)) ;;   # 不計輪、不計失敗
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
