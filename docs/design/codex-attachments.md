# Codex attachments

**Status:** approved design, 2026-11-23.

Files (rulebooks, handouts, sheets, music) can be attached to a codex page. They follow the page and its codex's rights, like pictures.

## Data
- `page_attachments (id, page_id, name, file, size, uploaded_by, created_at)`. `name` is the sanitised file name, `file` the random stored name. No removed rows and no versions.
- Foreign keys are off: deleting a page deletes its rows and files by hand. Moving a page takes them along.

## Rules
- Every page kind can have attachments: articles (journal entries included) and timelines.
- Anyone with See on the codex sees and downloads every attachment; uploading, replacing and removing need Edit. There are no hidden files.
- Types are an allow-list by extension, checked against the content where it can be: PDF, PNG/JPEG/WebP/GIF, Word/Excel/PowerPoint (doc, docx, xls, xlsx, ppt, pptx), OpenDocument (odt, ods, odp), txt, md, csv, zip, mp3. PDFs and images open in the browser, everything else downloads.
- At most 100 MB each, no total limit. The upload shows progress.
- Pictures and attachments of a page share one set of names (picture names count even when the picture was removed, because its text still renders); a clash gets ` (2)` before the extension.
- Remove deletes the row and the file for good, after a confirm. A link to it, in the page or an old revision, reads "attachment removed".
- Replace swaps the file and keeps the name, so links keep working. The new file must have the same extension; the old file is deleted.
- Uploading a name that was removed brings links to it back, since they go by name.
- Copying a page has "Copy attachments too" with the count and size, off by default. Ticked, every file is duplicated under a new random name.
- Attachments are not backed up. After a restore from backup the rows may point at files that are gone: the download is a 404 and the row stays until someone removes or replaces it.
- No notifications, no history entries, no graph edges.

## Text
- `![[rules.pdf]]` is a file link with the name and size; `![[rules.pdf|Player rules]]` uses the caption as the label. Only the page's own attachments resolve. Picture options mean nothing on a file.
- An unresolved embed whose name has a non-image attachment extension reads "attachment removed". One with an image extension keeps today's missing-picture text, since it could have been either.

## Pages
- A page has an Attachments box: name, size, who added it and when. Editors also get Add, Replace and Remove there; the editor has Insert. The box is hidden for readers when empty.
- A codex has an Attachments screen at `/codices/c/{id}/attachments` (a last segment, like `graph` and `inbox`): every file across its pages with its page, size and date. Read-only, for everyone with See.
- The game page gets nothing new.

*Later:* hiding a handout until it is revealed, an embedded PDF viewer, a link on the game page.
