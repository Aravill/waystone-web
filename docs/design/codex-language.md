# Codex language

A codex has a language, English or Czech. It decides the language of the built-in templates and of
the text the app writes into pages. The interface stays English.

## Data model

- `codices.language` (`en` or `cs`, `NOT NULL DEFAULT 'en'`, added by ALTER-if-missing). Existing
  codices are English.
- Languages are Go data (`codexLanguages`); adding one means a set of built-in templates, a date
  format and a journal folder name.

## Setting it

- New codex: the language has to be chosen, nothing is preselected. The API refuses a create without
  a known language (400).
- Edit codex: can be changed at any time by whoever can manage the codex (Edit Codices). An update
  without `language` leaves it as it is.
- Shown on the codex screen as a quiet label beside the name, only when it isn't English. Nowhere else.

## What follows the language

- **Only new text.** Pages, folders, tags, titles and history that already exist never change when
  the language does.
- **Built-in templates**: names, descriptions, text and tags. The ids (`builtin:session`…) are the same
  in every language, so folder defaults and the last used template keep working.
  - Czech names: Prázdná stránka, Sezení, Postava, Sídlo, Místo, Událost, Frakce, Předmět, Historie.
  - Czech tags: sezení, npc, sídlo, místo, událost, frakce, předmět, historie. A codex switched later can
    have both `event` and `událost`; merging is by hand.
- **Dates** (`{{date}}`, Today's date, the quick note stamp, a journal page's date, also in a codex's own
  templates): `Nov 23, 2026` or `23. 11. 2026`.
- **/ menu**: the list is a control, so its labels and search words stay English; the text it inserts is in the
  codex's language. Template rows use the template's name ("Sezení headings") and are also found by
  the built-in's English name.
- **Journal folder**: `{game}: Campaign Journal` or `{game}: Deník tažení`, from the codex's language when
  the folder is made. A game rename keeps renaming a folder that still has its generated name, in the language
  it was made in. The tree hides the `{game}: ` prefix for both. Page titles stay `{game}: YYYY-MM-DD`.
- **Create a page for it** on a timeline files the page in the top-level folder named Events or
  Události, whatever the language (the codex's own first).

## What stays English

Buttons, dialogs, "Wanted pages", live list boxes, "Session N", timeline labels, the writing guide, and the
history notes the app writes ("Added a quick note", "Copied from X", "Linked mentions of X",
"Picture layout"). The "(copy)" in a copied title too.

## Rules between flows

- Built-in names of **both** languages are reserved in every codex (409 `template_name_taken`), so
  a switch never makes two cards of one name. A template that already has such a name stays and can be edited
  while its name doesn't change.
- Copying or moving a page between codices of different languages translates nothing; tags that follow a copy stay
  as written.
- Games, journals and notes need nothing else: they read the language of the codex they write into.

## API

- `POST /api/codices` and `PUT /api/codices/c/{id}` take `language`; `GET /api/codices/c/{id}` sends `codex.language`.
- `GET /api/codices/templates/builtin?codex={id}` (See) returns that codex's `language`, `templates` and
  `today`; without `codex` it is English.

## Extras

- Demo data: the seeded codex is English.
- One E2E test: a Czech codex, a page from Sezení, then a switch to English.
