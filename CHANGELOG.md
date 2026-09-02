# Changelog

自動從 git 歷史產生（[Conventional Commits](https://www.conventionalcommits.org/) 格式的
commit subject，機械式一行一個改動）。**不要手動編輯這份檔案**——改用
`scripts/gen-changelog.sh` 重新產生，隨時可以重跑，整檔覆蓋。

想看「為什麼改、怎麼修的、以後怎麼避免」，見 [`LESSONS.md`](LESSONS.md)——
這份只列「改了什麼」。

## 2026-09-02

- fix(panel): schedule form — label-left, input-right row layout
- feat(panel): editable form for .ai/schedule.yml, replacing the read-only view
- fix(panel): consolidate remaining button styles into the .abtn component library
- fix(panel): make .row a flex container so .abtn buttons truly share alignment
- fix(panel): rename 立即中斷 to FORCE STOP for a clearer STOP/FORCE STOP pairing
- fix(panel): align .abtn button heights, color-code STOP vs 立即中斷
- docs: clarify .ai/STOP is a soft stop, not the source of orphaned tasks
- fix(panel): resolve claude CLI dir into launchd PATH, not just a hardcoded list
- fix(panel): replace emoji button icons with real SVG icons, unify button sizing
- feat(panel): move a quick STOP button next to the supervisor running row
- feat(panel): safe-idle kill switch for a running supervisor
- feat(panel): manage dev server launchd persistence from the panel UI
- feat(panel): add per-repo dev server launchd persistence
- fix(supervisor): review_after_task fallback now matches template default
- docs(supervisor): add schedule.yml key reference table

## 2026-09-01

- fix(panel): sort backlog by priority, not file order
- feat(panel): expose --quota-stop, --ignore-quota, --claude-flags

## 2026-08-31

- docs(panel): mention the backlog show-all toggle
- docs: convert ASCII-art diagrams to Mermaid
- feat(skills): add /pm-review — grounded gut-check on new ideas
- docs: position against Superlogical (terminal session persistence)

## 2026-08-29

- fix(runtime): session.lock staleness needs more than kill -0

## 2026-08-28

- fix(panel): let a card show its full backlog, not just the top 5
- fix(supervisor): don't flag BLOCKED as a checkpoint protocol violation
- docs(lessons): add CHANGELOG.md + LESSONS.md and their generators
- docs: reflect panel's supervisor-launch capability
- fix(supervisor): doctor no longer requires a dead Write(**) rule
- feat(runtime): session.lock for /ai-work and /ai-review invocations
- feat(panel): launch supervisor.sh from the control panel

## 2026-08-26

- fix(panel): don't count PAUSED placeholder HTML comment as a reply

## 2026-08-24

- feat(panel): status-grouped, collapsible repo list + schedule.yml viewer

## 2026-08-21

- feat(skills): add ai-healthcheck for pre-execution CONTRACT/backlog audits
- feat(templates): add content-completeness rubric so docs tasks get self-reviewed

## 2026-08-13

- feat(supervisor): make PAUSED poll interval configurable via schedule.yml

## 2026-08-07

- fix(panel): recompute LAN IP per request instead of at startup

## 2026-08-04

- chore: gitignore local Claude Code state
- feat(supervisor): make wait_on_pause configurable via schedule.yml

## 2026-07-30

- style(dashboard): move cumulative cost card to the rightmost position
- feat(dashboard): show project-wide estimated cost as the leading stat card
- fix(dashboard): move per-task cost from receipts to done-tasks table
- feat(dashboard): add per-task cost column to the receipts table
- fix(panel): fall back to task_id when a receipt has no title field

## 2026-07-29

- fix(skills): human-interactive skills honor the single-writer lock; track per-task cost
- fix(ai-init): interview also asks for the dev-server start command
- feat(supervisor): track token usage in events.jsonl, plan cost cross-validation
- feat(supervisor): add --quota-wait/--quota-stop for per-run overrides
- feat(panel): repo cards can start/stop their dev server

## 2026-07-24

- feat(panel): shippable count self-heals after a GitHub-side merge

## 2026-07-23

- fix(panel): stop misreading an unfilled reply template as answered
- feat(panel): repo cards can link out to a local dev-server URL
- fix(templates): drop dead Write() permission rules from settings.local.json

## 2026-07-22

- fix(ai-wrap): 盤點步驟加查 git stash list，避免 stash 藏成果被漏掉
- feat(dashboard): 收據列可點開看完整內文
- feat(panel): replace dashboard text link with an inline SVG icon button

## 2026-07-21

- feat(supervisor): --ignore-quota clears its own stale quota STOP
- feat(supervisor): --ignore-quota flag to intentionally burn quota for one run

## 2026-07-20

- rename(skills): /work → /ai-work, /review → /ai-review for naming consistency

## 2026-07-18

- feat(panel): add dashboard.html link to repo cards

## 2026-07-17

- docs(readme): sync durable-assets wording with the forced-review rule for large changes
- feat(skills): /ai-sync — pull initialized repos up to the latest templates
- feat(cost): raise default cost breaker to $20 and document the measured $2-5/task baseline
- feat(verify): repo-owned scripts/ai-verify.sh — the allowlisted end-to-end verification entry point
- feat(review): large changes (>10 files) force an independent review round instead of pausing

## 2026-07-16

- feat(panel): hot-reload the repo list — new repos appear within 5s, no restart
- feat(ai-init): auto-register new repo into ~/.aios-repos so panel and /ai-answer can see it
- docs(roadmap): V1 execution plan — zero-code Codex trial first, then an agent_command seam in the supervisor
- docs(runtime): headless write path verified on a real terminal — probe passed on momentum

## 2026-07-15

- docs(positioning): repository-first persistent operating layer — data/control plane split, minimum agent contract
- docs(roadmap): V1 milestone — second-agent (Codex) conformance run gates the agent-agnostic claim
- feat(supervisor): schedule-install.sh — launchd generator makes schedule.yml live up to its name
- feat(events): mechanical loop-level events.jsonl — supervisor emits, report/dashboard consume
- feat(supervisor): --doctor environment checkup + --probe headless write test
- fix(templates): allow Bash(date/git show) — timestamp protocol and /review silently broke without them
- feat(supervisor): structural lint for checkpoint/tasks state files — detect only, self-heal stays with /work
- docs(roadmap): vision-vs-reality audit, next-phase plan, deliberate non-goals
- feat(supervisor): recognize epoch/429/rate_limit_error limit variants — fixtures first, sleep straight to epoch
- docs(readme): positioning — a protocol layer, not an agent runtime
- feat(skills): /ai-answer — cross-repo PAUSED triage from the OS repo
- feat(panel): account usage in header, answered-PAUSED state, inputs survive re-render
- fix(supervisor): answered PAUSED no longer blocks startup — run a round to consume it
- fix(supervisor): quota gates — 5h only ever waits, only 7d can write STOP
- fix(protocol): timestamps must come from the date command, never guessed
- fix(protocol): clear PAUSED via mv — rm hard-deny deadlocked flag clearing

## 2026-07-10

- style(panel): wider cards — 480px min column, page up to 1720px
- fix(panel): buttons dead — quoted paths broke inline onclick attributes
- docs(manual): quota gates in supervisor flow diagram
- feat(supervisor): soft quota threshold — wait for reset instead of dying mid-task
- fix(supervisor): recognize session-limit messages and minute-precision reset times
- feat: interactive sessions join the audit trail — source field, /ai-wrap, intake rules, panel badges

## 2026-07-09

- feat: quota-aware supervisor brake + redesigned panel UI
- docs: tighten README intro (drop redundant paragraph)
- chore: scaffold standalone repo (README, CLAUDE.md, LICENSE, .gitignore) and drop work-record-tool self-references
- docs(aios): panel in README quick-start and MANUAL overview diagram
- feat(aios): local control panel + answerable-PAUSED protocol
- feat(aios): human-friendly interaction — /ai-task and /ai-answer
- docs(aios): MANUAL.md — operator's manual with flow diagrams
- feat(aios): Phase 4 — fresh-session review round, /ai-ship, static dashboard

## 2026-07-08

- fix(aios): apply all 9 adversarial-review findings
- docs(aios): first-run checklist + nested-session verification limits
- fix(aios): supervisor — brace vars before full-width punctuation (bash 3.2), verified full loop with stub claude
- docs(aios): CLAUDE.md routing, README quick start, headless Edit/Write allow rules in settings template
- feat(aios): Phase 2+3 — rubrics, personas, /ai-report, supervisor with recovery
- docs(aios): /ai-init accepts inline interview answers for headless install
