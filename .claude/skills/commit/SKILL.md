---
name: commit
description: Commit conventions for this repo. Use whenever creating git commits or naming branches/worktrees - small logical commits, short lower-case subjects without conventional-commit prefixes.
---

# commit

Only commit when the user has asked for it.

1. Commit in small, logical chunks; never bundle unrelated changes. Stage specific files rather than `git add -A`.
2. Subject: short, all lower case, at most 6 words (e.g. `add blocked field to user model`).
3. No conventional-commit prefixes in subjects (`fix:`, `feat:`, `chore:` etc.).
4. Put extra context in the commit body when needed.
5. Type prefixes belong on branch names (`feat/...`, `fix/...`, `chore/...`) and matching worktree names (`feat-...`, `fix-...`, `chore-...`).
