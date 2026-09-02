#!/usr/bin/env bash
# devserver-launchd-install.sh — 讓單一 repo 的 dev server 開機/登入自動
# 啟動，用 macOS 原生 launchd。只用 RunAtLoad（不用 KeepAlive）——常駐的
# 是「登入時幫你開好」，不是「你手動關掉它又幫你搶救回來」：panel 卡片
# 的 ■ 停止鍵殺掉 process 之後就是真的停了，不會被 launchd 立刻拉回來，
# 跟 panel/launchd-install.sh（panel 本體用 RunAtLoad+KeepAlive，因為那是
# 常駐服務、不是使用者會手動關掉的東西）刻意不同。
#
# pidfile／log 路徑跟 panel 自己 startDevServer() 用的是同一份（見
# panel/main.go devServerID／devPidFile／devLogFile 的 slug 規則）——所以
# 不管這個 process 是 launchd 開機時起的、還是你在 panel 卡片按「▶ 啟動」
# 起的，panel 都認得同一個 pid，卡片上的狀態燈、log 連結、■ 停止鍵完全
# 不用改一行程式碼就能管到它。
#
# 用法：
#   panel/devserver-launchd-install.sh --repo /path/to/repo
#   panel/devserver-launchd-install.sh --repo /path/to/repo --uninstall
#   panel/devserver-launchd-install.sh --repo /path/to/repo --status
#   panel/devserver-launchd-install.sh --repo /path/to/repo --dry-run
set -u

MODE=install
REPO=""
while [ $# -gt 0 ]; do
  case "$1" in
    --repo) REPO="$2"; shift 2 ;;
    --uninstall) MODE=uninstall; shift ;;
    --status) MODE=status; shift ;;
    --dry-run) MODE=dryrun; shift ;;
    *) echo "unknown flag: $1" >&2; exit 64 ;;
  esac
done

if [ -z "$REPO" ]; then
  echo "缺 --repo <path>" >&2
  exit 64
fi
if [ ! -d "$REPO" ]; then
  echo "找不到目錄：$REPO" >&2
  exit 66
fi
REPO="$(cd "$REPO" && pwd)"

REPOS_FILE="$HOME/.aios-repos"
if [ ! -f "$REPOS_FILE" ]; then
  echo "找不到 $REPOS_FILE" >&2
  exit 66
fi

# 找對應那一行，拆出第二欄（url，可空）、第三欄起（command，joined）——
# 跟 panel/main.go 的 loadDevConfig 同一套規則、同一個檔案來源。
LINE=""
while IFS= read -r raw; do
  line="$(printf '%s' "$raw" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  case "$line" in
    ""|"#"*) continue ;;
  esac
  set -- $line
  linepath="$(cd "$1" 2>/dev/null && pwd)"
  if [ "$linepath" = "$REPO" ]; then
    LINE="$line"
    break
  fi
done < "$REPOS_FILE"

if [ -z "$LINE" ]; then
  echo "~/.aios-repos 裡找不到這個 repo：$REPO" >&2
  exit 66
fi

set -- $LINE
if [ $# -lt 3 ]; then
  echo "這個 repo 在 ~/.aios-repos 沒有設定啟動指令（第三欄起）——先加上再裝這個" >&2
  exit 66
fi
shift 2
CMD="$*"

SLUG="$(printf '%s' "$REPO" | sed -E 's/[^A-Za-z0-9]+/_/g' | sed -E 's/^_+//;s/_+$//')"
LABEL="com.aios.devserver.${SLUG}"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
STATE_DIR="$HOME/.aios-panel-state"
PIDFILE="${STATE_DIR}/${SLUG}.pid"
LOGFILE="${STATE_DIR}/${SLUG}.log"

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
    echo "已移除 ${LABEL}（不會砍掉正在跑的 process——panel 卡片的 ■ 停止或自己 kill）"
    echo "下次登入不會再自動啟動這個 repo 的 dev server"
    exit 0 ;;
esac

mkdir -p "$STATE_DIR"

# 開機時先寫 pidfile（$$ 是這個 sh -c 的 pid），再 exec 換成真正的指令——
# exec 只換 process image、pid 不變，pidfile 全程有效，跟 panel 自己
# startDevServer() 寫 cmd.Process.Pid 是同一顆 pid 語意，panel 也才認得。
WRAPPED="echo \$\$ > '${PIDFILE}'; cd '${REPO}'; exec ${CMD}"

PLIST_BODY=$(cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>${LABEL}</string>
  <key>ProgramArguments</key><array>
    <string>/bin/sh</string>
    <string>-c</string>
    <string>${WRAPPED}</string>
  </array>
  <key>WorkingDirectory</key><string>${REPO}</string>
  <key>RunAtLoad</key><true/>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>StandardOutPath</key><string>${LOGFILE}</string>
  <key>StandardErrorPath</key><string>${LOGFILE}</string>
</dict></plist>
EOF
)

if [ "$MODE" = dryrun ]; then
  echo "dry-run：會寫入 $PLIST 並 bootstrap（RunAtLoad only，指令：${CMD}）"
  printf '%s\n' "$PLIST_BODY"
  exit 0
fi

launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
printf '%s\n' "$PLIST_BODY" > "$PLIST"
launchctl bootstrap "gui/$(id -u)" "$PLIST" || { echo "launchctl bootstrap 失敗" >&2; exit 1; }
echo "已安裝 ${LABEL}（開機/登入自動啟動這個 repo 的 dev server；沒有 KeepAlive——"
echo "panel 卡片的 ■ 停止鍵殺掉之後就是真的停了，不會被拉回來，下次登入才會再起）"
echo "  狀態：$0 --repo ${REPO} --status"
echo "  移除：$0 --repo ${REPO} --uninstall"
echo "  pid／log：跟 panel 卡片顯示的是同一份（${PIDFILE} / ${LOGFILE}）"
