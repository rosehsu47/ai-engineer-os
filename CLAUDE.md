# AI Engineer OS — Claude Code Context

This repo **is** the AI Engineer OS: a file protocol + skill set + supervisor
loop that lets a Claude Code agent work autonomously in any target repo —
persistent memory, checkpoint/resume, self-evaluation, audited receipts, and
crash/rate-limit recovery, all driven off files under `.ai/` in the target
repo (never in-process session state).

## Where things live

| File | Purpose |
|---|---|
| `AI-RUNTIME.md` | Protocol spec — schemas, `AIOS_STATUS`, error recovery |
| `ROADMAP.md` | Vision-vs-reality audit + next-phase plan and the deliberate non-goals |
| `MANUAL.md` | Operator's manual with flow diagrams |
| `supervisor/README.md` | The unattended loop (`supervisor.sh`) — recovery, cost breaker, kill switch |
| `panel/README.md` | Local web control panel — multi-repo status, inline answers, STOP button |
| `templates/` | Single source of truth for what gets copied into target repos — fix templates here, not per-repo copies |

## Skills shipped in this repo (`.claude/skills/`)

| Skill | What it does |
|---|---|
| `/ai-init` | Installs the `.ai/` Agent Runtime Workspace into a target repo (copies templates, interviews the user to fill the CONTRACT, wires up permissions) |
| `/ai-report` | Aggregates a target repo's `.ai/receipts` into daily/weekly reports (PR description draft, changelog, résumé material) |
| `/ai-ship` | Pushes a target repo's `ai/queue` branch to GitHub and opens/updates a PR (description auto-generated from receipts) — external action, human-triggered only |
| `/ai-answer` | Scans every repo in `~/.aios-repos` for `.ai/PAUSED`, surfaces all pending questions, and walks the user through answering each one (writes `## 人類回覆` sections — never deletes `PAUSED` or edits task files itself) |

## Key rules (always apply)

- **`templates/`** is the single source of truth copied into target repos —
  fix bugs/behavior there, never in a repo that already ran `/ai-init`
- **Schema and protocol questions are settled by `AI-RUNTIME.md`** — don't
  invent new frontmatter fields or state files without updating it there first
- **`/ai-ship` is the only skill that touches the network** (`git push`,
  `gh pr create/edit`) — it is human-triggered only; the supervisor loop
  never calls it
- **Single-writer invariant**: only one agent session writes to a target
  repo's `.ai/` state at a time — this is what makes checkpoint/resume safe;
  don't design around parallel writers
