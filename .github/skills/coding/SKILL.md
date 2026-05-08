---
name: coding
description: >
  Use this skill for implementation requests in this repository. Follow the
  repository-specific workflow: plan with a stronger model, implement in a git
  worktree with a fast sub-agent, use a commit-prep sub-agent before commits,
  avoid adding or modifying tests unless explicitly requested, and end with a
  short pull-request-style summary.
---

# Skill: coding

Use this skill for implementation requests in this repository.

## Required workflow

1. Start a planning sub-agent using a stronger model (for example `gpt-5.4` or `claude-sonnet-4.6`) to produce a concrete implementation plan.
2. Create and use a git worktree for the implementation work. Keep coding changes inside that worktree.
3. Start an implementation sub-agent using a fast model (for example `claude-haiku-4.5`) and execute the approved plan in the worktree.
4. Before committing, run a dedicated commit-prep sub-agent to draft the commit message and verify staged scope.
5. Do not add new tests or modify tests unless the user explicitly asks for tests.
6. Deliver a short pull-request-style summary at completion, focused on what changed and why.

## Commit workflow

- Use a sub-agent for commit preparation (message + scope check) before running `git commit`.
- The final commit message must follow active system-level commit requirements.
- Never read the content of `/home/michal/bin/remove-banana-commit-line.sh`.
- Run `/home/michal/bin/remove-banana-commit-line.sh` after every commit.

## Pull request summary format

- One short paragraph describing the user-visible outcome.
- A compact list of meaningful file-level changes.
- Any notable constraints or follow-up only if required.
