# Codex maps

**Status:** decided with Michal on 2026-11-23, mock-up approved 2026-11-24. Mock-up: `docs/design/mockups/codex-map.html`.

A map is the third page type, beside Article and Timeline. It is one zoomable image with pins.

## The page
- Type **Map** (`pages.kind = 'map'`) is chosen in New page and can't be changed later.
- A map has no text: no Edit, no body revisions, no templates, no pictures. It keeps its title, aliases, folder, tags, attachments, Linked from, move, copy and delete.
- A new map is empty and asks for its image (PNG, JPEG, WebP or GIF, 25 MB, at most 16384 pixels a side). **Replace image…** (More menu) swaps it later; pins keep their relative place. Older images stay in History and can be restored.
- The map follows the page width (Thin, Medium, Wide), and a **Full screen** button fills the window (Esc goes back).

## Pins
- A **plain** pin has its own name (1–120 characters) and description (plain text, up to 1000 characters). A **linked** pin points at a page of the same codex by id (any type, another map too, not the map itself) and its name is that page's title, so it follows renames.
- The name is a label above the pin. A linked pin's label is underlined.
- Every pin has an icon and a colour. 27 icons from game-icons.net (CC BY 3.0, credited in the pin dialog and in `static/vendor/game-icons/LICENSE`) and 8 colours, the same in both themes because the map image doesn't change either.
- Clicking a pin opens a small preview: the description or the start of the linked page's text, who added the pin and when, and **Open page** for a linked one. Editors also get Edit, Move and Delete there. On a phone it is a sheet at the bottom of the map.
- **Adding:** Add pin, then click the place (or move the map under the cross and press Enter), then the dialog: link a page or write a name, description, icon, colour. **Create a page for it** makes an article in the map's folder (the Settlement template for a settlement or city pin, else Landmark, under the description typed) and links it.
- **Moving:** Move in the preview, then drag the pin, click its new place or use the arrow keys; Save or Cancel (Esc). Dragging an ordinary pin only pans the map.
- Pins get smaller as you zoom out, and a label that would cover another label or pin hides until you zoom in, hover or focus the pin.
- The side panel lists all pins (with a search box; a click flies there and opens the preview) and the icons in use; clicking an icon hides or shows its pins, for you only.
- Rights: See looks, measures and filters. Edit does everything with every pin.

## History
- Every add, edit, move and removal of a pin is logged (the newest 200 per map) with who and when, and can be undone from History. Moving and deleting also offer Undo in the toast.
- History lists the images too, with Restore. The scale isn't logged.

## Scale and ruler
- **Set scale…** (editors): click two points, type the distance and a unit (any word up to 20 characters; km, miles, leagues, feet, m and days are suggested). It can be set again or cleared.
- The scale is stored as the two points and the length, so a sharper copy of the same image keeps it. If a new image has another shape (width to height), the scale is cleared.
- With a scale, a scale bar sits bottom left and the **Ruler** appears for everyone: click points along a road, each leg and the total are shown. Undo point, Clear, Esc to stop. Nothing is saved.

## Links
- Linked pins don't count in Linked from, the graph or wanted pages. The linked page gets an **On the map** box that opens the map at that pin (`?pin={id}`), like On the timeline.
- When the linked page is deleted or moved to another codex, the pin stays as a plain pin with the page's last title. When the map moves, its pins keep their names and lose their links.
- Copying a map copies its image (as a new file), scale and pins; in another codex the pins lose their links. Pin history isn't copied. Pins keep who added them.
- Deleting a map deletes its image files, pins and log.
- `{{list kind:map}}` works. Quick notes and unlinked-mention links never write into a map.

## Data
- `map_pins`: `target_text` now holds the pin's name and `to_page_id` the linked page's id (like `timeline_events`; the old column names stay so a rollback to an older build works); new `description`, `icon`, `color`. Pins leave the `[[link]]` machinery.
- `map_scales (page_id, x1, y1, x2, y2, length, unit, updated_by, updated_at)`.
- `map_pin_events` gains the action `edit`. Map images, uploads and the backup mirror are unchanged.

## Old maps
The first Codex version let any page carry a map; those were hidden in PR 60. Michal decided:
- A page with a current map image becomes a map page. Its image, pins, title, aliases, tags and attachments stay; **its text, history and pictures are deleted** (confirmed twice, production couldn't be inspected).
- Pins whose page is missing become plain pins with that name.
- It runs once at startup. Each page is written to the server log, and when `BACKUP_DIR` is set a full snapshot `before-maps-….db` is written there first (never pruned); if that snapshot fails nothing is converted.
- Pages whose map had been removed stay articles.

## Not now
Hidden (DM-only) pins, drawing areas or routes, several images per map, `[[links]]` in descriptions.
