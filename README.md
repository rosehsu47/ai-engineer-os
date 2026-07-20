# AI Engineer OS

**The persistent operating layer for coding agents.** Agent state belongs
in the repository, not in the conversation: install a file protocol + skill
set into any target repo, and the repo itself carries the contract, task
queue, memory, checkpoints, and audited receipts an agent needs to work
unattended — plus a supervisor loop for crash/rate-limit recovery, quota
braking, a local web control panel, and a kill switch. **Operator's manual
with flow diagrams: [`MANUAL.md`](MANUAL.md)**; protocol spec:
[`AI-RUNTIME.md`](AI-RUNTIME.md); vision audit and next phase:
[`ROADMAP.md`](ROADMAP.md).

```bash
# 1. Install the runtime into a target repo (interview fills the CONTRACT)
/ai-init /absolute/path/to/repo

# 2. Seed tasks in {repo}/.ai/tasks/backlog.yaml, then run one iteration:
cd /path/to/repo && claude -p "/ai-work"

# 3. Or run unattended (rate-limit auto-resume, cost breaker, kill switch):
supervisor/supervisor.sh --repo /path/to/repo
touch /path/to/repo/.ai/STOP     # brake at any time

# 4. Turn receipts into reports / PR descriptions / résumé material:
/ai-report /path/to/repo weekly
```

Receipts are structured enough to feed downstream reporting/résumé tooling
if you build one, but that's outside this repo's scope.

## Positioning: a repository-first persistent operating layer

Most coding-agent setups keep the working state in the conversation: what's
done, why it changed, where to resume, why it stopped. When the chat ends,
the agent forgets. This project moves that state into the repository —
**the conversation is just UI; the repository is the state** — so an agent
session is stateless and disposable, and crash / rate-limit / kill-switch
recovery is the same code path as a normal start.

It is **not** an agent framework, workflow engine, planner, or tool-calling
layer — it never decides how the agent thinks (today all of that is
delegated to Claude Code via `claude -p "/ai-work"`). What it maintains is the
working environment every agent run shares, all as files under `.ai/`:

```
         Loop engine（supervisor / cron / CI —— replaceable）
                          │
                          ▼
              ┌──────────────────────────┐
              │      AI Engineer OS      │
              ├──────────────────────────┤
              │  Contract      State     │  data plane —
              │  Memory        Tasks     │  what the agent
              │  Context       Receipts  │  reads and writes
              ├──────────────────────────┤
              │  Control（human）        │  control plane —
              │   STOP · PAUSED          │  authority the human keeps,
              │   schedule / budget      │  deny-listed from the agent
              └──────────────────────────┘
                          │
                          ▼
                   Git repository
```

The **data plane** is the agent's working environment: contract (rules,
boundaries, definition of done), execution state (checkpoint / resume),
long-term memory and decisions, the task queue, project context, and
receipts (evidence for every change). The **control plane** is the
authority the human keeps: `STOP` (a kill switch any loop must respect),
`PAUSED` (the approval-boundary round-trip), and `schedule.yml` (budget,
quota thresholds, start times — deny-listed so the agent cannot reschedule
or re-budget itself).

Quota braking shows how the layers split: the OS defines the *vocabulary*
(a STOP flag, threshold keys, `quota_wait`/`quota_stop` events) and the
loop implements the *policy* (wait out the 5h window, stop on the 7d
window). Swap the loop; the vocabulary still holds.

The durable assets, in order of value:

1. **Contract with hard approval boundaries** (`CONTRACT.md` §7) — new
   dependencies, schema migrations, ambiguous acceptance criteria: the
   agent must write `.ai/PAUSED` and stop. Large changes (>10 files)
   don't pause — they raise a flag that forces an independent review
   round instead. Not a prompt-level suggestion — the file protocol and
   permission deny-rules enforce it.
2. **Receipt-centric evidence discipline** — every task ends in a receipt
   with acceptance-by-acceptance verification, test output, self-eval rubric
   scores, and known limitations. Receipts are the single source for PR
   descriptions (`/ai-ship`), reports/changelogs/résumé material
   (`/ai-report`), and independent review rounds. The agent's work is only
   as real as its evidence.
3. **Control-plane vocabulary** — STOP / PAUSED / schedule, including
   subscription-quota braking (soft: wait for the 5h window to reset;
   hard: stop to preserve the human's weekly quota). Token-billed
   frameworks don't have this problem; subscription CLI users do, and no
   framework we know of handles it.
4. **Single-writer file state** — one agent writes `.ai/` at a time. This
   is what makes checkpoint/resume trustworthy, and it's why parallel
   multi-agent writers are deliberately not built (see below).

### What's replaceable

- **Agent** — designed so any coding agent (Claude Code, Codex, Cursor,
  Gemini CLI, whatever comes next) can pick up the same `.ai/` workspace.
  **Honest status: a design goal, not yet a verified claim.** The protocol
  assumes nothing Claude-specific except the skill-loading mechanism, but
  no second agent has driven it yet — see the minimum agent contract in
  [`AI-RUNTIME.md`](AI-RUNTIME.md) and the Codex conformance milestone in
  [`ROADMAP.md`](ROADMAP.md) (V1).
- **Loop** — turn-based today (`supervisor.sh`); goal-based, time-based,
  or CI-triggered loops can drive the same files.
- **Scheduler** — launchd today (`schedule-install.sh`); cron, GitHub
  Actions, or Claude Code's native scheduled routines would do equally.
- **Storage is deliberately *not* on this list** — plain files in git are
  an architectural choice, not a limitation awaiting a database: diffable,
  auditable, portable, and readable by any agent without a driver.

### vs. general agent frameworks (e.g. Nous Research's [hermes-agent](https://github.com/nousresearch/hermes-agent))

Hermes is a full standalone runtime: its own loop, 40+ tools over RPC,
messaging gateways (Telegram/Discord/Slack/…), model-agnostic backends,
self-improving skills. It answers *"how do I run a persistent general
assistant across all my platforms?"* This project answers a narrower
question — *"how do I let a coding agent work unattended in **this repo**
without losing auditability, my approval authority, or my quota?"* — and
the pieces that answer it (contract boundaries, PAUSED human-reply
protocol, receipts, quota braking, git discipline on an `ai/queue` branch)
are precisely the pieces a general assistant framework doesn't center.
Different layer, different problem; using Hermes would still leave you
building everything in the numbered list above.

The honest overlap risk is not Hermes but **Claude Code's own evolution**:
background tasks, scheduled cloud routines, and task queues are growing
natively. The supervisor shell loop and web panel are the most replaceable
parts of this repo, and that's fine — they're thin. The protocol and the
evidence discipline are the parts worth carrying forward, potentially even
onto other CLI agents (the `.ai/` protocol assumes nothing Claude-specific
except the skill format).

### Development direction

- **Invest**: receipt schema and its downstream consumers (reports, PR
  bodies, review verdicts), contract/approval vocabulary, recovery paths.
  These compound and nothing else provides them.
- **Keep thin**: supervisor loop, panel, dashboards — treat them as
  disposable shells around the protocol, ready to be swapped for native
  Claude Code features as they mature.
- **Don't build**: parallel writers, messaging gateways, model routing,
  a tool ecosystem — other layers own those problems.

Phase 4 extensions:

```bash
supervisor/supervisor.sh --repo /path --review   # fresh-session review after each task
supervisor/dashboard.sh --repo /path             # static HTML dashboard (zero quota)
/ai-ship /path                                   # push ai/queue + GitHub PR (human-triggered)
```

Human interaction layer (no YAML editing needed):

```bash
/ai-task 一句話描述想做什麼      # guided task creation: choices + drafted acceptance
/ai-answer                       # answer a PAUSED agent via multiple choice
cd panel && go run . -repos /a,/b   # local control panel: http://127.0.0.1:7777
                                    # multi-repo status, inline answers, STOP buttons
```

Manual with flow diagrams: [`MANUAL.md`](MANUAL.md) · Supervisor details:
[`supervisor/README.md`](supervisor/README.md) · Panel details:
[`panel/README.md`](panel/README.md)

Deliberately NOT built: parallel multi-agent writers — that would break the
single-writer invariant that makes checkpoint/resume safe. The review round
is a separate *reader* session, which is where a second agent actually adds
value.

## License

MIT — see LICENSE.
