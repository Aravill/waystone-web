---
name: design-grill
description: Grill Michal with design questions before implementing a new feature in Waystone Web. Use whenever a new feature or a significant change to an existing one is requested (a new entity, flow, page, permission or notification), or when asked to "grill me", "design this first" or "tune out edge cases". Explores how the app works, then asks rounds of pointed FE/BE questions about how the feature fits with campaigns, sign-ups, sessions, systems, notifications and deletion, and writes the decisions to docs/design/ before any code.
---

# design-grill

Goal: settle edge cases in design, not in error states after shipping. Michal doesn't
write the code, so the questions are about product behaviour; translate each one into
its BE/FE consequence in the option descriptions.

## 1. Learn the ground first (no questions yet)

Read just enough to ask questions only this app could raise:
- `CLAUDE.md` (architecture gotchas), `docs/mvp.md`, and the schema in `db/sqlite_store.go`.
- Every existing flow the feature touches: models, handlers and the pages that show them.
  Usual suspects: sign-ups (`api/signups.go`: Pending → Accepted/Declined/Kicked,
  leave/withdraw), admin add/kick player (`api/admin.go`), campaign edit and status
  transitions (`api/campaigns.go`, `models/campaign.go`), systems (`api/systems.go`:
  free-text campaign system, rename cascades, delete), account anonymization
  (`AnonymizeUser`), notifications (`models/notification.go`), the game page
  company panel/chronicle (`static/game.*`), profile, and the Home agenda.
- Note the *existing* answers to similar questions (e.g. "foreign keys are off, so
  cascades are by hand", "everyone logged in can see profiles") so options can mirror them.

Tell Michal in one line what you looked at, then start asking.

## 2. Grill in rounds

Use `AskUserQuestion`, up to 4 questions per round, usually 5–7 rounds. Each question:
- Stands alone: names the concrete flow or state ("Roy is kicked from Strahd while Kael
  is assigned…"), not an abstract property.
- Has 2–4 options, the recommended one first and marked "(Recommended)". Each
  description states the user-visible behaviour **and** the cost (new state, new UI,
  per-viewer filtering, a notification type…).
- Offers a "keep it simple" option whenever one exists. Michal wants the app minimal.

Order of rounds (skip what doesn't apply):
1. **Core model & entry point**: when the thing is created or attached, whether it's
   required (hard vs. optional with a nudge), cardinality, and what it's keyed on.
2. **Interaction with existing rules**: free-text vs. known values, blank/custom
   legacy data, what happens when the *other* side changes (a campaign's system or
   status edited, a system renamed or deleted, a player leaving, kicked or rejoining,
   a game finished or cancelled).
3. **Lifecycle**: edit, swap, delete while in use (block / archive / cascade),
   history vs. clearing, account deletion (anonymize keeps the user row), admin
   deleting a campaign.
4. **Permissions & visibility**: owner / DM / admin, *which* DM (active vs. any),
   which fields each can touch, who can read it (everyone / members / owner).
5. **UI placement**: nav page vs. profile section vs. game page dialog, URLs, empty
   states, where the nudge or pending action appears (game card, game page, Home agenda).
6. **Notifications**: which events notify whom (multiSelect), and reusing existing
   types before adding new ones.
7. **Scope extras**: seed demo data, an E2E test (only stable, high-value flows),
   `.env.example` if there's config.

## 3. Hunt contradictions

After each round, check the answers against each other and against existing rules
before moving on. Typical collisions:
- "Block X changing while Y is assigned" in one place, but "allow freely" for the
  mirror-image change elsewhere.
- "Kept as history" plus "delete blocked only if active": say what deletion does to
  the history.
- A new free-form attribute (like a status) that overlaps with an earlier lifecycle
  decision: ask whether it's a label or has effects.
- Mid-grill additions from Michal: fold them in with their own follow-up questions.

Ask a dedicated follow-up for each collision. Never resolve one silently. When an
answer is free text, read it literally: it often adds a rule ("tell the admin to
migrate instead") or a note for later.

## 4. Write it down, then build

- Write `docs/design/<feature>.md` in the feature's worktree: data model, rules grouped
  by flow, notifications, pages/API, scope extras, and "*Later:*" notes Michal deferred.
  Use the decisions only, not the Q&A transcript. Commit and push it as the first commit.
- Small mechanical calls nobody was asked about (e.g. same-day tie-breaks) go in the
  doc as stated decisions and get mentioned in the final summary.
- Then hand off to the `coding` skill, with the design doc as the spec.
