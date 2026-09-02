#!/usr/bin/env bash
# launchd-install.sh — 讓 aios-panel 開機/登入自動啟動、當掉自動重開，
# 用 macOS 原生 launchd（RunAtLoad + KeepAlive），不是排程時刻——這跟
# supervisor/schedule-install.sh 是不同性質：那支是「固定時刻啟動一輪」，
# 這支是「常駐服務，掛了就重開」。panel 是單例（整台機器一份，不分 repo），
# 所以不需要 --repo，plist 用固定 label com.aios.panel。
#
# 用法：
#   panel/launchd-install.sh              # 安裝/更新，之後開機/登入自動啟動
#   panel/launchd-install.sh --uninstall  # 移除（不會砍掉正在跑的 process）
#   panel/launchd-install.sh --status     # 看目前載入狀態
#   panel/launchd-install.sh --dry-run    # 只印 plist 不動系統
set -u

MODE=install
while [ $# -gt 0 ]; do
  case "$1" in
    --uninstall) MODE=uninstall; shift ;;
    --status) MODE=status; shift ;;
    --dry-run) MODE=dryrun; shift ;;
    *) echo "unknown flag: $1" >&2; exit 64 ;;
  esac
done

LABEL="com.aios.panel"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
PANEL_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$PANEL_DIR/.." && pwd)"
BIN="$HOME/bin/aios-panel"
STATE_DIR="$HOME/.aios-panel-state"

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
    echo "已移除 ${LABEL}（正在跑的 process 不會被砍，要停用 panel 卡片的 STOP／自己 kill）"
    exit 0 ;;
esac

if [ ! -x "$BIN" ]; then
  echo "找不到 ${BIN}——先 build：" >&2
  echo "  cd $PANEL_DIR && go build -o $BIN ." >&2
  exit 66
fi

mkdir -p "$STATE_DIR"

PLIST_BODY=$(cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>${LABEL}</string>
  <key>ProgramArguments</key><array>
    <string>${BIN}</string>
    <string>-dashboard-script</string>
    <string>${ROOT_DIR}/supervisor/dashboard.sh</string>
    <string>-supervisor-script</string>
    <string>${ROOT_DIR}/supervisor/supervisor.sh</string>
    <string>-devserver-launchd-script</string>
    <string>${ROOT_DIR}/panel/devserver-launchd-install.sh</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>StandardOutPath</key><string>${STATE_DIR}/launchd.log</string>
  <key>StandardErrorPath</key><string>${STATE_DIR}/launchd.log</string>
</dict></plist>
EOF
)

if [ "$MODE" = dryrun ]; then
  echo "dry-run：會寫入 $PLIST 並 bootstrap（RunAtLoad+KeepAlive，binary：${BIN}）"
  printf '%s\n' "$PLIST_BODY"
  exit 0
fi

launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
printf '%s\n' "$PLIST_BODY" > "$PLIST"
launchctl bootstrap "gui/$(id -u)" "$PLIST" || { echo "launchctl bootstrap 失敗" >&2; exit 1; }
echo "已安裝 ${LABEL}（開機/登入自動啟動，當掉 launchd 會自動重開）"
echo "  狀態：$0 --status"
echo "  移除：$0 --uninstall"
echo "  log：${STATE_DIR}/launchd.log"
echo "  repo 清單讀 ~/.aios-repos（熱重載，不用重啟 job 就能加新 repo）"
