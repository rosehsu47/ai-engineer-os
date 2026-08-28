#!/usr/bin/env bash
# scripts/gen-changelog.sh — 用 Conventional Commits 格式的 git log 機械式
# 重新產生 CHANGELOG.md，不需要手動維護。冪等：每次都整檔重寫，隨時重跑
# 都會跟目前的 git 歷史同步。零外部依賴（純 git + bash，macOS bash 3.2
# 相容——這個 repo 的慣例，見 supervisor.sh 開頭註解）。
#
# 前提：commit subject 用 Conventional Commits 格式
# （`type(scope): summary`）。merge commit 自動跳過（--no-merges）；
# 不符合格式的 subject 也跳過，不塞進 changelog 製造雜訊。
#
# 用法：scripts/gen-changelog.sh
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="CHANGELOG.md"
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

{
  echo "# Changelog"
  echo
  echo "自動從 git 歷史產生（[Conventional Commits](https://www.conventionalcommits.org/) 格式的"
  echo "commit subject，機械式一行一個改動）。**不要手動編輯這份檔案**——改用"
  echo "\`scripts/gen-changelog.sh\` 重新產生，隨時可以重跑，整檔覆蓋。"
  echo
  echo "想看「為什麼改、怎麼修的、以後怎麼避免」，見 [\`LESSONS.md\`](LESSONS.md)——"
  echo "這份只列「改了什麼」。"

  last_date=""
  git log --no-merges --date=short --pretty=format:'%ad%x09%s' | while IFS="$(printf '\t')" read -r date subject; do
    # 過濾掉不是 conventional commit 格式的 subject（例如舊歷史裡偶爾漏規範的），
    # 不強行塞進 changelog 製造雜訊——這類 commit 保留在 git log 裡，只是不進 changelog。
    if ! printf '%s\n' "$subject" | grep -qE '^[a-z]+(\([A-Za-z0-9_./-]+\))?!?: '; then
      continue
    fi
    if [ "$date" != "$last_date" ]; then
      echo
      echo "## $date"
      echo
      last_date="$date"
    fi
    echo "- $subject"
  done
} > "$TMP"

mv "$TMP" "$OUT"
trap - EXIT
n=$(git log --no-merges --oneline | wc -l | tr -d ' ')
echo "已重新產生 ${OUT}（掃了 $n 筆非 merge commit）"
