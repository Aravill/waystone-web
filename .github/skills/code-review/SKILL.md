---
name: code-review
description: >
  Use this skill for code reviews in this repository. Reviews latest changes
  using a moderately strong model, identifies issues and improvements, presents
  findings, and optionally invokes the coding skill to implement improvements
  while skipping the planning phase.
---

# Skill: code-review

Use this skill to review code changes in this repository.

## Required workflow

1. Start a code-review sub-agent using a moderately strong model (for example `claude-sonnet-4.6`) to analyze changes.
2. The review agent examines latest changes using `git diff HEAD` and recent commit history.
3. The review agent produces a structured output:
   - A numbered list of issues and suggested improvements
   - Each item includes: file path, line number(s), issue type, description, and specific suggestion
4. The review agent **makes no code changes** during the review phase.
5. Present findings to the user and ask whether they want to implement the improvements.
6. If the user agrees to implement improvements:
   - Invoke the coding skill (`.github/skills/coding/SKILL.md`)
   - Pass the review output as context to skip the planning phase (planning is already done by the review)
   - Let the coding skill implement the improvements in a worktree
7. If the user declines, the review workflow is complete.

## Output format

Review findings should be presented as a numbered list:

```
1. **[File path]** (line X-Y)
   - Type: [bug|security|performance|style|logic]
   - Issue: [Description of the problem]
   - Suggestion: [Specific improvement or fix]

2. **[File path]** (line X-Y)
   - Type: [bug|security|performance|style|logic]
   - Issue: [Description of the problem]
   - Suggestion: [Specific improvement or fix]
```

## When to use this skill

- Reviewing code changes before merging
- Analyzing recent commits for issues or improvements
- Performing security, performance, or style audits
- Validating implementation against project conventions
