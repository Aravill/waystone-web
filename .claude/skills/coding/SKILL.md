---
name: coding
description: Implementation workflow for this repo. Use for any feature, fix, or change request - plan with a Plan subagent, implement in a git worktree under .claude/worktrees with an implementation subagent, keep changes minimal, and finish with a short PR-style summary.
---

# coding

1. **Plan.** If the user supplied a plan, use it as-is and skip this step. Otherwise launch a `Plan` subagent (default model) to produce a concrete plan: files to touch, existing helpers to reuse, routes to register in `api/router.go`, Store interface changes. Show the plan to the user before implementing if it's non-trivial.
2. **Worktree.** Create the worktree under `.claude/worktrees/` only, named after the branch with its type prefix (branch `feat/session-export` → `.claude/worktrees/feat-session-export`), branched from `main`:
   `git worktree add .claude/worktrees/<type>-<name> -b <type>/<name> main`
3. **Implement.** Launch a `general-purpose` subagent with `model: "sonnet"` to execute the plan inside the worktree. Give it the plan plus the relevant CLAUDE.md conventions. Use `model: "haiku"` for trivial, mechanical changes.
4. **Rules for the implementation:**
   - Minimal, surgical changes; no broad refactors unless asked.
   - DRY: reuse existing helpers and patterns first; add an abstraction only when it clearly removes duplication.
   - Do not add or modify tests unless the user explicitly asks.
   - UI work follows `STYLE_GUIDE.md`.
5. **Verify** in the worktree: `go build ./... && go vet ./...`.
6. **Do not commit.** Wait until the user asks (then use the `commit` skill) or commits themselves.
7. **Finish** with a short pull-request-style summary: what changed, why, and anything the user should check manually (e.g. pages to click through).
