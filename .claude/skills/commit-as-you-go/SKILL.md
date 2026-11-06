---
name: commit-as-you-go
description: When to commit and push in this repo. Use during any work that changes files - commit and push each finished logical chunk right away, without being asked, instead of saving it all for the end.
---

# commit-as-you-go

Commit and push each logical chunk as soon as it is finished. Don't wait until the end of the work, and don't wait to be asked. Use the `commit` skill for message and branch conventions.

1. **Start on a branch.** Never commit directly to `main`. Branch from the latest main: `git fetch origin`, then branch from `origin/main`.
2. **Find the chunks before you start.** A chunk does one thing and can be described in one short subject, such as a fix, a feature's backend, or its page. Unrelated changes never share a commit.
3. **Check, then commit.** Run `go build ./... && go vet ./...` when Go changed. Each commit should leave the app building and working. Stage specific files, and specific hunks when a file holds two chunks.
4. **Push right after each commit, and open a PR with the first push.** Every new branch gets a pull request, without being asked; give the user the link. Later commits go to the same PR. Run the E2E tests when UI behaviour changes, and before saying the work is done.
5. **Changing what's already committed.** To reword or redo something, add a commit on top, revert it, or rebase. After a rebase, push with `--force-with-lease`, and only to your own feature branch. Never rewrite `main`.

Merging stays with the user.
