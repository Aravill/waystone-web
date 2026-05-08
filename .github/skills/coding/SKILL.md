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
4. Do not commit changes, wait until the user asks for it explicitly, or does it themselves
5. Do not add new tests or modify tests unless the user explicitly asks for tests.
6. Deliver a short pull-request-style summary at completion of the changes













