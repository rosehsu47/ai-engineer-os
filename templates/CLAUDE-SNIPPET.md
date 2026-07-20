<!-- aios-intake-rules（/ai-init 安裝；靠這行 marker 判斷已安裝，勿改動） -->
## AI Engineer OS — 互動 session 規則

本 repo 由 AI Engineer OS 管理：`.ai/tasks/` 是任務佇列、`.ai/receipts/`
是稽核收據（協定見 ai-engineer-os/AI-RUNTIME.md）。互動對話中適用：

- **Intake**：使用者提出新功能/修改需求時，先確認一次「現在直接做，
  還是排進 backlog 讓 /ai-work 自動迴圈跑？」——偏大、可獨立驗收、不急的
  工作建議用 `/ai-task` 種進 backlog，而不是直接動手。
- **收尾**：互動中直接改了程式碼，告一段落時必須執行 `/ai-wrap`：
  commit + receipt（`source: human-interactive`）+ done.yaml 條目。
  工作不留在 working tree 不記帳——沒收據的工作在 panel 與
  /ai-report 上等於不存在。
