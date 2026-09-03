# shared/models.md — Claude API 定價參考

> 給 `supervisor.sh` P1（訂閱制下的真實成本可見度，見 `ROADMAP.md` §3）
> 用的定價來源：拿 `events.jsonl` 記錄的 token 用量，自己重新算一次成本，
> 跟 claude CLI 自報的 `total_cost_usd` 對照，抓出差距。**這份檔案只是
> 定價數字的參考表，不是程式碼會自動讀取的設定檔**——P1 真正的交叉驗算
> 邏輯還沒寫（見 ROADMAP §3），寫的時候手動把這裡的數字抄進 `supervisor.sh`。

**資料來源與新鮮度**：數字取自 `/claude-api` skill 內建的定價表（該 skill
標記快取時間 2026-06-24）與其 `shared/prompt-caching.md` 的
「Economics」小節，兩者都是 Anthropic 官方一手資料的快取版本，不是這個
repo 自己量測的。**定價會隨時間變動**——用這份檔案寫 P1 的交叉驗算邏輯
前，先跑一次 `/claude-api` 確認數字沒有過期；長期沒人跑 `/ai-work` 的
repo，這份檔案也可能悄悄跟真實定價脫節，不要假設它永遠正確。

## Base input/output 定價（每 1M token，USD）

| Model | Model ID | Input | Output |
|---|---|---|---|
| Claude Opus 5 | `claude-opus-5` | $5.00 | $25.00 |
| Claude Sonnet 5 | `claude-sonnet-5` | $2.00 | $10.00 |
| Claude Haiku 4.5 | `claude-haiku-4-5` | $1.00 | $5.00 |
| Claude Opus 4.8 | `claude-opus-4-8` | $5.00 | $25.00 |
| Claude Opus 4.7 | `claude-opus-4-7` | $5.00 | $25.00 |
| Claude Opus 4.6 | `claude-opus-4-6` | $5.00 | $25.00 |
| Claude Sonnet 4.6 | `claude-sonnet-4-6` | $3.00 | $15.00 |
| Claude Fable 5 / Claude Fable 5.1 | `claude-fable-5` / `claude-fable-5-1` | $10.00 | $50.00 |

`panel` 的「啟動 supervisor」表單與 `supervisor.sh --model` 目前只開放
`opus`/`sonnet`/`haiku` 三個值（見 `panel/README.md`），實際解析成哪個
`claude-*` model ID 由 claude CLI 自己決定，不是這個協定層寫死的——上表
的 Opus 5／Sonnet 5／Haiku 4.5 三列是目前最可能對應到的版本，但**不要
硬編碼假設**，claude CLI 版本更新時預設模型可能換代。

## Prompt caching 倍率

以 base input 價格為基準：

- **Cache write**：5 分鐘 TTL ≈ **1.25×**；1 小時 TTL ≈ **2×**
- **Cache read**：≈ **0.1×**（Claude Fable 5.1／Mythos 5.1 是例外，
  ≈0.025×／$0.25 每 MTok；本專案預設用 opus/sonnet/haiku，不受此例外
  影響）

對應 `events.jsonl` 的 `iteration` 事件欄位（見 `AI-RUNTIME.md` 事件
模型節）：

```
estimated_cost_usd
  = input_tokens          × (Input 定價 / 1,000,000)
  + output_tokens         × (Output 定價 / 1,000,000)
  + cache_creation_input_tokens × (Input 定價 / 1,000,000) × 1.25   # 假設 5 分鐘 TTL
  + cache_read_input_tokens     × (Input 定價 / 1,000,000) × 0.1
```

跟 claude CLI 自報的 `total_cost_usd` 對不上時，P1 要標記「這輪成本是
估算值，跟獨立試算差距 X%」，而不是沉默地二選一相信誰（見 `ROADMAP.md`
§3 P1 的完整理由）。

## 已知限制

- 這份表沒有涵蓋 Batch API（半價）、Priority Tier、fast mode 等特殊計價
  路徑——`supervisor.sh` 目前呼叫 `claude -p` 走的是標準計價，用不到這些。
- 5 分鐘 vs 1 小時 cache TTL 只有 write 倍率不同（1.25× vs 2×），
  `events.jsonl` 目前不記錄 TTL 是哪一種，上面公式先假設 5 分鐘 TTL
  （claude CLI 目前實際用哪種需要再確認，這裡沒有驗證來源，是這份文件
  唯一還沒查證的假設）。
