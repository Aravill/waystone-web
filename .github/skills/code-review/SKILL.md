---
name: code-review
description: >
  Use this skill to review commit diffs with a multi-pass sub-agent workflow
  and produce a prioritized issue list.
---

# Skill: code-review

Use this skill when reviewing code changes in this repository.

## Required workflow

1. Review the target commit diffs in multiple iterations.
2. First iteration: use a fast model to summarize each change and its intended effect.
3. Second iteration: validate expected behavior with sub-agents.
4. In the second iteration, use fast models for small files with few changed lines and slower, smarter models for larger or complex diffs.
5. Third iteration: run a final once-over using a medium model (for example Sonnet) to catch missed issues.
6. Verify correctness against intent, consistency across code, DRY opportunities, modularity, separation into layers, and whether existing code should be reused instead of adding duplicate logic.
7. Check architecture and design fit for a lightweight, fast, minimalistic app.
8. Return findings as a list of issues sorted by priority in descending order (highest priority first).
9. If no issues are found, explicitly state that no actionable issues were found.
