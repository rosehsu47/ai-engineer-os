# AI Engineer OS

A generic autonomous Claude Code agent runtime: install a protocol + skill
set into any target repo, then run it unattended via a supervisor loop with
crash/rate-limit recovery, checkpoint/resume, audited receipts, and a local
web control panel.

Beyond a one-off `claude` session, this repo ships an agent runtime that lets
a Claude Code agent work **autonomously** in any target repo — with persistent
memory, checkpoint/resume, self-evaluation, audited receipts, and
crash/rate-limit recovery. **Operator's manual with flow diagrams:
[`MANUAL.md`](MANUAL.md)**; protocol spec: [`AI-RUNTIME.md`](AI-RUNTIME.md).

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
