# AI Engineer OS

A generic autonomous Claude Code agent runtime: install a protocol + skill
set into any target repo, then run it unattended via a supervisor loop with
persistent memory, checkpoint/resume, self-evaluation, audited receipts, a
local web control panel, and crash/rate-limit recovery. **Operator's manual
with flow diagrams: [`MANUAL.md`](MANUAL.md)**; protocol spec:
[`AI-RUNTIME.md`](AI-RUNTIME.md).

```bash
# 1. Install the runtime into a target repo (interview fills the CONTRACT)
/ai-init /absolute/path/to/repo

# 2. Seed tasks in {repo}/.ai/tasks/backlog.yaml, then run one iteration:
cd /path/to/repo && claude -p "/work"

# 3. Or run unattended (rate-limit auto-resume, cost breaker, kill switch):
supervisor/supervisor.sh --repo /path/to/repo
touch /path/to/repo/.ai/STOP     # brake at any time

# 4. Turn receipts into reports / PR descriptions / résumé material:
/ai-report /path/to/repo weekly
```

Receipts are structured enough to feed downstream reporting/résumé tooling
if you build one, but that's outside this repo's scope.

## Positioning: a protocol layer, not an agent runtime

This project does **not** implement an agent loop, tool execution, or model
routing — all of that is delegated to Claude Code (`claude -p "/work"`).
What it implements is the layer Claude Code doesn't give you: **how a
subscription-billed CLI agent becomes an accountable, resumable, resident
engineer inside a specific repo.** Everything lives in files under `.ai/`;
an agent session is stateless and disposable, so crash / rate-limit /
kill-switch recovery is the same code path as a normal start.

The durable assets, in order of value:

1. **Contract with hard approval boundaries** (`CONTRACT.md` §7) — new
   dependencies, schema migrations, >10 files touched, ambiguous acceptance
   criteria: the agent must write `.ai/PAUSED` and stop. Not a prompt-level
   suggestion — the file protocol and permission deny-rules enforce it.
2. **Receipt-centric evidence discipline** — every task ends in a receipt
   with acceptance-by-acceptance verification, test output, self-eval rubric
   scores, and known limitations. Receipts are the single source for PR
   descriptions (`/ai-ship`), reports/changelogs/résumé material
   (`/ai-report`), and independent review rounds. The agent's work is only
   as real as its evidence.
3. **Subscription-quota awareness** — dual-threshold braking (soft: wait
   for the 5h window to reset; hard: stop to preserve the human's weekly
   quota). Token-billed frameworks don't have this problem; subscription
   CLI users do, and no framework we know of handles it.
4. **Single-writer file state** — one agent writes `.ai/` at a time. This
   is what makes checkpoint/resume trustworthy, and it's why parallel
   multi-agent writers are deliberately not built (see below).

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
