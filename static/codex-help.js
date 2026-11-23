// The writing guide at /codices/help: sections of static intro HTML plus examples. Each example's
// `src` is shown as written and rendered through the real codexMarkdown (see codexHelpHtml in codex.js),
// so what the guide shows is what a page does. A section with `snippets: true` also gets one example
// per entry of the editor's / menu (CODEX_SNIPPETS).
const CODEX_HELP = [
    {
        id: 'text',
        title: 'Text',
        intro: '<p>Pages are written in a small, safe subset of Markdown. A blank line starts a new paragraph and a single line break stays a line break. Anything else you type is shown as typed, so HTML tags do nothing. There are no tables, code blocks, strikethrough or nested lists.</p>',
        examples: [
            { src: 'The mill burned down in the spring.\nNobody saw who lit it.\n\nBy summer the miller was gone.', note: 'Paragraphs and line breaks' },
            { src: 'The **Order of the Lantern** keeps *watch* over the pass, or so _they_ claim.', note: '**bold** and *italic* or _italic_ (bold may contain italic)' },
            { src: '# The Pass\nA short summary under the heading.\n\n## Dangers\n\n### Rockfalls\nWatch the upper road.', note: '# and ## make a banded heading, ### a smaller one. A heading goes on the first line of its paragraph.' },
            { src: '- Rope\n- Lantern oil\n- A map nobody trusts\n\n1. Climb\n2. Rest\n3. Descend', note: 'Lists start with -, * or 1. (one level only)' },
            { src: '> "Never trust a quiet river."\n> Old miller\'s saying', note: 'Quotes start with >' },
            { src: 'Read the [travellers\' guide](https://example.com/guide) before you go.', note: 'Links to the web: http, https or mailto only. They open in a new tab.' },
            { src: 'Before the storm.\n\n---\n\nAfter the storm.', note: 'A line of three dashes alone is a divider' }
        ]
    },
    {
        id: 'links',
        title: 'Linking pages',
        intro: '<p>Put a page\'s title or alias in double brackets to link to it. Links reach only the pages of the same codex, ignore capital letters and need the title on one line (no <code>[</code>, <code>]</code> or <code>|</code> inside it). A link to a page that does not exist yet is drawn dashed: click it to create the page, or find it under Wanted pages. Linked pages show the link in their "Linked from" box.</p>' +
            '<ul><li>Type <code>[[</code> in the editor and pick a page from the suggestions.</li>' +
            '<li>Select some words and press Ctrl+K (Cmd+K on a Mac) or the Link button. They become a link to the page of that title, or to a new page of that name.</li>' +
            '<li>After you save a page with links to pages that do not exist, a dialog offers to create them in one go.</li></ul>',
        examples: [
            { src: 'The [[Old Mill]] stands by the river.', note: 'A link to a page' },
            { src: 'Ask at [[The Gilded Stag|the inn]] about the [[Old Mill|mill]].', note: 'After a bar, the text shown instead of the title' },
            { src: 'Nobody has written [[The Hollow King]] yet.', note: 'A page that does not exist yet' }
        ]
    },
    {
        id: 'people',
        title: 'Mentioning people',
        intro: '<p>Type <code>@</code> in the editor and pick a user, or write their whole username, like <code>@roy</code>. It becomes a mention that keeps pointing at the same person whatever they call themselves later, and readers see their current name. Click it to open their profile. The <code>@</code> must be at the start of a line or after a space. Nobody is notified.</p>',
        examples: [
            { src: 'The map was drawn by @[Roy](roy).', note: 'A mention as the text keeps it, with the id of the user in round brackets' }
        ]
    },
    {
        id: 'pictures',
        title: 'Pictures',
        intro: '<p>Add a picture with the Add image button above the editor, by dropping a file on it or by pasting one (PNG, JPEG, WebP or GIF, up to 25 MB). The editor inserts <code>![[name]]</code> for you. Write the embed by hand to place a picture that is already on the page.</p>' +
            '<p>After the name you can add options and a caption, split by <code>|</code>. Options are words from <b>small, medium, large, full</b> (size) and <b>left, center, right</b> (spot). The defaults are medium and right, and full is always centred. Options only matter for a picture alone on its line; text then wraps around it. Several pictures on lines of their own in a row form a gallery, and a picture inside a line of text is shown small, inline. When you can edit a page, a Layout button on each picture changes its size and spot for you.</p>',
        examples: [
            { src: '![[map.png]]\n\nText wraps around a medium picture on the right.', note: 'Just the picture' },
            { src: '![[map.png|The old road]]\n\nThe words after the name are the caption.', note: 'A caption' },
            { src: '![[map.png|small left|The old road]]\n\nA small picture on the left, with a caption.', note: 'Options, then a caption' },
            { src: '![[map.png|full|The valley at dawn]]', note: 'Full width, centred' },
            { src: '![[map.png|The north gate]]\n![[map.png|The south gate]]', note: 'Pictures one after another form a gallery' },
            { src: 'The seal ![[map.png]] was pressed into red wax.', note: 'Inside a line of text' }
        ]
    },
    {
        id: 'attachments',
        title: 'Attached files',
        intro: '<p>Files such as rulebooks, handouts and sheets are added in the Attachments box on a page (people with Edit only; PDF, images, Word, Excel, PowerPoint, OpenDocument, txt, md, csv, zip and mp3, up to 100 MB each). Write <code>![[rules.pdf]]</code> to link one in the text: it shows the name and the size. Words after the name become the label. PDFs and images open in the browser, everything else downloads.</p>' +
            '<p>A file name can only be used once on a page, pictures included. If the file has been removed, the text says "Attachment removed" with its name; uploading a file of that name again brings the link back.</p>',
        examples: [
            { src: 'Read ![[rules.pdf]] before the first session.', note: 'A file link' },
            { src: 'Read ![[rules.pdf|Player rules]] before the first session.', note: 'With a label' }
        ]
    },
    {
        id: 'lists',
        title: 'Live lists',
        intro: '<p>A line of its own made of <code>{{list ...}}</code> shows a box of matching pages that stays up to date as the codex changes. Options are <code>key:value</code> pairs (put a value in "quotes" if it has spaces), and with none the list shows every page. The page itself is left out. Lists are drawn when the page is shown, so they add no links, backlinks or graph lines.</p>' +
            '<ul><li><code>tag:name</code> only pages with that tag. Repeat it and a page must have all of them.</li>' +
            '<li><code>folder:Name</code> pages in any folder of that name, sub-folders included. <code>folder:Places/Inns</code> follows a path from the top.</li>' +
            '<li><code>links:here</code> pages that link to this one.</li>' +
            '<li><code>kind:lore</code> or <code>kind:timeline</code></li>' +
            '<li><code>sort:title</code> (the default), <code>sort:updated</code> or <code>sort:created</code> (newest first)</li>' +
            '<li><code>limit:10</code> 1 to 200, 50 if you do not say. The box then says how many more there are.</li></ul>' +
            '<p>An option the list does not know is reported in the box. The examples below use made-up pages.</p>',
        examples: [
            { src: '{{list}}', note: 'Every page' },
            { src: '{{list tag:npc}}', note: 'Pages with a tag' },
            { src: '{{list folder:Places sort:updated limit:2}}', note: 'A folder, newest first, at most two' },
            { src: '{{list folder:Places/Inns}}', note: 'A folder path' },
            { src: '{{list kind:timeline}}', note: 'Only timelines' }
        ]
    },
    {
        id: 'templates',
        title: 'Templates',
        intro: '<p>A new page can start from a template: nine built-in ones (Session, Character, Settlement and so on) and any your codex has saved. Choose one in the New page dialog. Two placeholders are filled in once, when the page is made: <code>{{title}}</code> becomes the page\'s title and <code>{{date}}</code> the day, written like "Nov 22, 2026". Nothing else in braces changes.</p>' +
            '<ul><li>To make a template, use "Save as template" in a page\'s More menu, or Templates in the codex\'s left nav (people with Edit only).</li>' +
            '<li>A folder can start its new pages from a template: open the folder\'s menu and choose "New pages start from…". Pages made inside it, or in its sub-folders, preselect that template.</li></ul>',
        examples: [
            { src: '# {{title}}\n\nWritten on {{date}}.', note: 'A template body. The page "The Old Mill" gets this text with the title and day filled in.' }
        ]
    },
    {
        id: 'menu',
        title: 'The / menu',
        intro: '<p>While writing, type <code>/</code> at the start of a line or after a space to open a menu of snippets, then keep typing to narrow it down and press Enter. The Insert / button above the editor opens the same menu. Below are the snippets, with what each puts in the page. The menu also offers today\'s date and, for every template that has <code>##</code> headings, "{template name} headings", which inserts just those headings.</p>',
        snippets: true,
        examples: []
    },
    {
        id: 'keys',
        title: 'Shortcuts',
        intro: '<ul><li><kbd>/</kbd> jumps to the search box, when you are not typing in a field.</li>' +
            '<li><kbd>n</kbd> opens a quick note, when you can edit and are not typing in a field.</li>' +
            '<li><kbd>Ctrl</kbd>+<kbd>K</kbd> (<kbd>Cmd</kbd>+<kbd>K</kbd> on a Mac) in the editor links the selected words.</li>' +
            '<li><kbd>Esc</kbd> closes menus and dialogs.</li></ul>',
        examples: []
    }
];
