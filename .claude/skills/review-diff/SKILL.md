---
name: review-diff
description: Multi-pass subagent review of commit diffs in this repo, producing a priority-sorted issue list. Use when asked to review a commit, branch, or set of changes against this project's design goals.
---

# review-diff

Target: `$ARGUMENTS` (commit range, branch, or PR). If empty, review `main...HEAD`, or uncommitted changes if HEAD is `main`.

1. **Pass 1 - summarize.** Launch a subagent with `model: "haiku"` to summarize each changed file: what changed and its intended effect.
2. **Pass 2 - validate behavior.** Launch subagents in parallel to check each change does what it intends. Use `model: "haiku"` for small files with few changed lines; use `model: "opus"` for large or complex diffs (auth, DB layer, routing, session logic).
3. **Pass 3 - once-over.** Launch one subagent with `model: "sonnet"` over the whole diff to catch anything the earlier passes missed.
4. Across passes, check:
   - correctness against intent;
   - consistency with existing code and CLAUDE.md conventions;
   - DRY: duplicated logic, existing helpers that should have been reused;
   - modularity and layering (handlers in `api/`, persistence via the `db` Store interface, models in `models/`);
   - fit for a lightweight, fast, minimalistic app (no unnecessary dependencies or abstractions).
5. Verify each finding yourself before reporting; drop anything you can't substantiate.
6. Output a list of issues sorted by priority, highest first, each with `file:line`, the problem, and a suggested fix. If nothing survives, say explicitly that no actionable issues were found.
