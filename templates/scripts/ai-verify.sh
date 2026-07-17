#!/usr/bin/env bash
# ai-verify.sh — repo 自己的驗證腳本（AIOS 慣例）
#
# 為什麼存在：agent 在 headless 下的 Bash 白名單是 prefix 比對，
# `curl -s -X POST http://...` 這種帶 flag 的形態不會命中
# `Bash(curl http://localhost:*)`——與其窮舉 curl 變體，不如把驗證
# 能力收進這一個腳本：白名單只放行 `bash scripts/ai-verify.sh`，
# 腳本裡面要 curl、要 playwright、要打本機服務都隨你。
#
# 契約整合：CONTRACT §5（DoD）要求本腳本存在時必須執行且通過；
# agent 會把輸出摘錄進 receipt 的證據節。
#
# 寫法約定：
# - 每個檢查失敗要 exit 非零、印出失敗原因（agent 據此修正）
# - 保持冪等、可重複執行；不要有互動式 prompt
# - 需要 dev server 的檢查：自己啟動、自己清理（或檢查已在跑）
set -u

echo "ai-verify: 尚未定義任何驗證步驟（模板骨架）"
echo "  範例（取消註解並改成你的服務）："
echo "  # curl -sf http://localhost:3000/api/health || { echo 'health check 失敗'; exit 1; }"
echo "  # curl -sf -X POST http://localhost:3000/api/echo -d '{\"ping\":1}' | grep -q pong || exit 1"
exit 0
