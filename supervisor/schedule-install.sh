#!/usr/bin/env bash
# schedule-install.sh — 讀目標 repo schedule.yml 的 schedule_start_times，
# 產生並載入 macOS launchd job，讓 supervisor 在固定時刻自動啟動。
# launchd 是作業系統原生排程：睡醒補跑、重開機存活；supervisor 既有的
# 安全閥（lock 防重疊、STOP/PAUSED、quota 門檻、max_cost）對排程啟動
# 的 run 全部有效——`touch .ai/STOP` 永遠贏過排程。
#
# 用法：
#   schedule-install.sh --repo /path/to/repo             # 安裝/更新 job
#   schedule-install.sh --repo /path/to/repo --uninstall # 移除 job
#   schedule-install.sh --repo /path/to/repo --status    # 看載入狀態
#   schedule-install.sh --repo /path/to/repo --dry-run   # 只印 plist 不動系統
#   schedule-install.sh --self-test                      # 零額度驗證時間解析
set -u

REPO="" MODE=install
while [ $# -gt 0 ]; do
  case "$1" in
    --repo) REPO="$2"; shift 2 ;;
    --uninstall) MODE=uninstall; shift ;;
    --status) MODE=status; shift ;;
    --dry-run) MODE=dryrun; shift ;;
    --self-test) MODE=selftest; shift ;;
    *) echo "unknown flag: $1" >&2; exit 64 ;;
  esac
done

parse_times() { # $1 = "09:00,21:30" → stdout 每行 "H M"；任一 token 非法 → return 1
  local t h m
  for t in $(printf '%s' "$1" | tr ',' ' '); do
    case "$t" in
      ([0-9]:[0-9][0-9]|[0-9][0-9]:[0-9][0-9]) : ;;
      (*) echo "非法時間 token：${t}（要 HH:MM，例 09:00）" >&2; return 1 ;;
    esac
    h=$((10#${t%%:*})); m=$((10#${t##*:}))
    { [ "$h" -le 23 ] && [ "$m" -le 59 ]; } || { echo "非法時間：$t" >&2; return 1; }
    printf '%s %s\n' "$h" "$m"
  done
}

if [ "$MODE" = selftest ]; then
  pass=0 fail=0
  ok() { pass=$((pass+1)); echo "  ok  $1"; }
  bad() { fail=$((fail+1)); echo "  FAIL $1"; }
  [ "$(parse_times '09:00,21:30' 2>/dev/null)" = "9 0
21 30" ] && ok "兩個時刻解析" || bad "兩個時刻解析"
  parse_times '25:00' >/dev/null 2>&1 && bad "拒絕 25:00" || ok "拒絕 25:00"
  parse_times '9:5' >/dev/null 2>&1 && bad "拒絕 9:5（分鐘要兩位）" || ok "拒絕 9:5（分鐘要兩位）"
  [ "$(parse_times '09:00 21:30' 2>/dev/null | wc -l | tr -d ' ')" = 2 ] \
    && ok "空白分隔也容忍（逐 token 驗證）" || bad "空白分隔也容忍（逐 token 驗證）"
  echo "self-test: pass=$pass fail=$fail"; [ "$fail" = 0 ]; exit $?
fi

[ -n "$REPO" ] || { echo "--repo 必填（或用 --self-test）" >&2; exit 64; }
[ -d "$REPO/.ai" ] || { echo "$REPO/.ai 不存在——先跑 /ai-init" >&2; exit 66; }
REPO=$(cd "$REPO" && pwd)   # launchd 需要絕對路徑

SLUG=$(basename "$REPO" | tr 'A-Z' 'a-z' | tr -cd 'a-z0-9-')
LABEL="com.aios.supervisor.${SLUG}"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
SUP="$(cd "$(dirname "$0")" && pwd)/supervisor.sh"

case "$MODE" in
  status)
    if launchctl list 2>/dev/null | grep -q "$LABEL"; then
      echo "已載入：$LABEL"
      launchctl list "$LABEL" 2>/dev/null | grep -E 'PID|LastExitStatus' || true
      echo "plist：$PLIST"
    else
      echo "未載入：${LABEL}（plist $( [ -f "$PLIST" ] && echo 存在但未載入 || echo 不存在 )）"
    fi
    exit 0 ;;
  uninstall)
    launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
    rm -f "$PLIST"
    echo "已移除 $LABEL"
    exit 0 ;;
esac

TIMES_RAW=$(grep -E '^schedule_start_times:' "$REPO/.ai/schedule.yml" 2>/dev/null | head -1 \
  | sed -E 's/^[^:]+:[[:space:]]*//; s/[[:space:]]*#.*$//; s/^"(.*)"$/\1/; s/[[:space:]]*$//')
if [ -z "$TIMES_RAW" ]; then
  echo "schedule.yml 的 schedule_start_times 為空——沒有要排程的時刻。" >&2
  echo "先在 $REPO/.ai/schedule.yml 設定，例：schedule_start_times: \"09:00,21:30\"" >&2
  exit 65
fi
TIMES=$(parse_times "$TIMES_RAW") || exit 65

# launchd 的 PATH 極簡（/usr/bin:/bin:...），claude CLI 多半不在——把安裝
# 當下解析到的 claude 位置塞進 job 的 PATH，supervisor 才叫得到它。
CLAUDE_DIR=$(dirname "$(command -v claude 2>/dev/null || echo /usr/local/bin/claude)")
JOB_PATH="${CLAUDE_DIR}:/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"

CAL_ENTRIES=$(printf '%s\n' "$TIMES" | while read -r h m; do
  printf '    <dict><key>Hour</key><integer>%s</integer><key>Minute</key><integer>%s</integer></dict>\n' "$h" "$m"
done)

PLIST_BODY=$(cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>${LABEL}</string>
  <key>ProgramArguments</key><array>
    <string>/bin/bash</string>
    <string>${SUP}</string>
    <string>--repo</string>
    <string>${REPO}</string>
  </array>
  <key>StartCalendarInterval</key><array>
${CAL_ENTRIES}  </array>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>${JOB_PATH}</string>
  </dict>
  <key>StandardOutPath</key><string>${REPO}/.ai/supervisor/launchd.log</string>
  <key>StandardErrorPath</key><string>${REPO}/.ai/supervisor/launchd.log</string>
</dict></plist>
EOF
)

if [ "$MODE" = dryrun ]; then
  echo "dry-run：會寫入 $PLIST 並 bootstrap（時刻：${TIMES_RAW}）"
  printf '%s\n' "$PLIST_BODY"
  exit 0
fi

mkdir -p "$HOME/Library/LaunchAgents" "$REPO/.ai/supervisor"
launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
printf '%s\n' "$PLIST_BODY" > "$PLIST"
launchctl bootstrap "gui/$(id -u)" "$PLIST" || { echo "launchctl bootstrap 失敗" >&2; exit 1; }
echo "已安裝 ${LABEL}（時刻：${TIMES_RAW}）"
echo "  狀態：$0 --repo $REPO --status"
echo "  移除：$0 --repo $REPO --uninstall"
echo "  隨時煞車：touch $REPO/.ai/STOP（排程照觸發，但 supervisor 見 STOP 即退）"
