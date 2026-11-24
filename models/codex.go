package models

// Page kinds. A lore page is an article; a timeline page also lists dated events (timeline_events);
// a map page is an image with pins (map_images, map_pins) and has no text.
const (
	PageLore     = "lore"
	PageTimeline = "timeline"
	PageMap      = "map"
)

// ValidNewPageKind tells whether a page can be created with this kind.
func ValidNewPageKind(k string) bool { return k == PageLore || k == PageTimeline || k == PageMap }

// Codex is a folder of Codex pages. Games point at a codex with campaigns.codex_id (a label only).
type Codex struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Language    string `json:"language"` // LangEnglish or LangCzech
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// A codex's language decides the language of its built-in templates and of the text written into its
// pages (docs/design/codex-language.md).
const (
	LangEnglish = "en"
	LangCzech   = "cs"
)

// ValidLanguage tells whether a codex can have this language.
func ValidLanguage(l string) bool { return l == LangEnglish || l == LangCzech }

// Folder organises the pages of one codex into a nested tree. It is for sorting only: links,
// titles and aliases ignore it.
type Folder struct {
	ID       string `json:"id"`
	CodexID  string `json:"-"`
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
	// DefaultTemplate: "" none, "builtin:<id>", or a template id of the same codex.
	DefaultTemplate string `json:"default_template"`
	CreatedBy       string `json:"-"`
	CreatedAt       string `json:"-"`
	UpdatedAt       string `json:"-"`
}

// PageContributor is one person's credit on a page. It outlives pruning of old revisions.
type PageContributor struct {
	UserID        string
	Revisions     int
	FirstRevision int
}

// Page is a Codex page with its newest revision.
type Page struct {
	ID              string
	Kind            string
	CodexID         string
	FolderID        string // "" = the codex's top level
	SessionID       string // the game session this page is the journal of, or ""
	Title           string
	AuthorID        string
	CreatedAt       string
	UpdatedAt       string
	Aliases         []string
	Body            string
	CurrentRevision int
	RevisionCount   int // revisions still kept
	UpdatedByID     string
	Contributors    []PageContributor // ordered by first revision
}

// PageListItem is a page without its body, for lists and link resolution in the client.
type PageListItem struct {
	ID        string
	Kind      string
	CodexID   string
	FolderID  string
	Title     string
	Aliases   []string
	CreatedAt string
	UpdatedAt string
}

type PageRevision struct {
	Number       int    `json:"number"`
	AuthorID     string `json:"author_id"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	Note         string `json:"note"`
	RestoredFrom int    `json:"restored_from"`
	CreatedAt    string `json:"created_at"`
}

// RecentRevision is one line of a "recent changes" list.
type RecentRevision struct {
	PageID    string
	Title     string
	Kind      string
	CodexID   string
	Number    int
	Note      string
	AuthorID  string
	CreatedAt string
}

type PageSaveOutcome string

const (
	PageSaved     PageSaveOutcome = "saved"
	PageUnchanged PageSaveOutcome = "unchanged"
	PageStale     PageSaveOutcome = "stale"
)

type PageSaveResult struct {
	Outcome  PageSaveOutcome
	Revision PageRevision
	Latest   *PageRevision // set when stale
}

// PageLink is one distinct [[target]] in a page's body. ToPageID "" means no page yet.
type PageLink struct {
	FromPageID string
	TargetText string
	ToPageID   string
}

// PageBacklink is a page that links to a page.
type PageBacklink struct {
	PageID     string
	Title      string
	Kind       string
	TargetText string
}

// WantedPage is a title that links point at but no page has yet.
type WantedPage struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

type MapImage struct {
	ID         string `json:"id"`
	PageID     string `json:"page_id"`
	File       string `json:"file"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	UploadedBy string `json:"uploaded_by"`
	CreatedAt  string `json:"created_at"`
}

// MapPin sits at x,y (fractions of the image). A linked pin shows its page's title; Name then keeps
// the last known title for when the link is gone.
type MapPin struct {
	ID           string  `json:"id"`
	MapPageID    string  `json:"map_page_id"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	LinkedPageID string  `json:"linked_page_id"` // "" = a plain pin
	Icon         string  `json:"icon"`
	Color        string  `json:"color"`
	CreatedBy    string  `json:"created_by"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// PinIcons and PinColors are the known pin looks, the first one the default (static/codex-pins.js has
// the drawings and labels).
var (
	PinIcons = []string{"place", "settlement", "city", "fortress", "tower", "inn", "temple", "shop", "port", "bridge", "mine",
		"camp", "battle", "dragon", "monster", "danger", "dungeon", "cave", "ruins", "tomb", "treasure", "portal",
		"forest", "mountain", "quest", "party", "person"}
	PinColors = []string{"red", "orange", "gold", "green", "teal", "blue", "purple", "black"}
)

// MapScale says that the two points (fractions of the image) are Length Units apart.
type MapScale struct {
	X1     float64 `json:"x1"`
	Y1     float64 `json:"y1"`
	X2     float64 `json:"x2"`
	Y2     float64 `json:"y2"`
	Length float64 `json:"length"`
	Unit   string  `json:"unit"`
}

// MapCopy is what a copied map page starts with.
type MapCopy struct {
	Image *MapImage // already pointing at its own copied file
	Scale *MapScale
	Pins  []MapPin // ids are replaced
}

// MapPinEvent logs a pin change. Before and After are JSON MapPin snapshots ("" when absent).
type MapPinEvent struct {
	ID        string
	MapPageID string
	PinID     string
	UserID    string
	Action    string // add | edit | move | remove
	Before    string
	After     string
	UndoneBy  string
	CreatedAt string
	Name      string // the pin's name in the snapshot
}

// Per-user rights (user_rights.action). Admins always have every right.
const (
	RightCodicesSee  = "codices.see"
	RightCodicesEdit = "codices.edit"
)

// UserRights lists the known per-user rights.
var UserRights = []string{RightCodicesSee, RightCodicesEdit}

// Per-codex access levels (codex_access.level).
const (
	CodexLevelSee  = "see"
	CodexLevelEdit = "edit"
)

// CodexAccess is one user's level on one codex.
type CodexAccess struct {
	CodexID string `json:"codex_id"`
	UserID  string `json:"user_id"`
	Level   string `json:"level"`
}

// CodexTag is a label that belongs to one codex; PageCount is filled when tags are listed.
type CodexTag struct {
	ID        string `json:"id"`
	CodexID   string `json:"codex_id,omitempty"`
	Name      string `json:"name"`
	PageCount int    `json:"page_count"`
	CreatedBy string `json:"-"`
	CreatedAt string `json:"-"`
}

// PageImage is a picture uploaded into a page. RemovedAt is set when it is taken out of the
// editor's list; the row and the file stay so old revisions that mention it still show it.
type PageImage struct {
	ID         string `json:"id"`
	PageID     string `json:"page_id"`
	Name       string `json:"name"`
	File       string `json:"file"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	UploadedBy string `json:"uploaded_by"`
	CreatedAt  string `json:"created_at"`
	RemovedAt  string `json:"removed_at"`
}

// PageAttachment is a file attached to a page (the file itself lives in the attachments directory).
// Replace keeps the row and its id and swaps File, Size, UploadedBy and CreatedAt.
type PageAttachment struct {
	ID         string `json:"id"`
	PageID     string `json:"page_id"`
	Name       string `json:"name"`
	File       string `json:"file"`
	Size       int64  `json:"size"`
	UploadedBy string `json:"uploaded_by"`
	CreatedAt  string `json:"created_at"`
}

// CodexAttachment is a page attachment with the title of the page it is on, for the codex-wide list.
type CodexAttachment struct {
	PageAttachment
	PageTitle string `json:"page_title"`
}

// CodexTemplate is a page template that belongs to one codex. Tags are the tags a page made from
// it gets (they exist in the codex).
type CodexTemplate struct {
	ID          string     `json:"id"`
	CodexID     string     `json:"codex_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Body        string     `json:"body"`
	Tags        []CodexTag `json:"tags"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

// NewPageExtras is what a new page gets besides its text: a revision note, tags (by name; the
// codex's missing ones are created), pictures (rows already pointing at their own copied files)
// and which of those pictures is the cover.
type NewPageExtras struct {
	Note         string
	TagNames     []string
	Images       []PageImage
	Attachments  []PageAttachment
	CoverImageID string
	Calendar     *TimelineCalendar // timeline pages: the copied calendar
	Events       []TimelineEvent   // timeline pages: the copied events (ids are replaced)
	Map          *MapCopy          // map pages: the copied image, scale and pins
	NoteID       string            // a quick note filed as this page: deleted in the same transaction
}

// CodexNote is a private quick note in one user's inbox of a codex.
type CodexNote struct {
	ID        string `json:"id"`
	CodexID   string `json:"codex_id"`
	UserID    string `json:"user_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// TimelineEvent is one dated entry on a timeline page. Month and Day are 0 when unknown. A linked
// event shows its page's title; Title keeps the last known title for when the link is gone.
type TimelineEvent struct {
	ID           string
	PageID       string
	Title        string
	LinkedPageID string // "" = no page
	Year         int
	Month        int
	Day          int
	CreatedBy    string
	CreatedAt    string
	UpdatedAt    string
}

// TimelineCalendar names a timeline's months (one per entry; none = plain numbers) and its eras.
type TimelineCalendar struct {
	Months    []string `json:"months"`
	Era       string   `json:"era"`
	BeforeEra string   `json:"before_era"`
}

// GraphPage is one page of a codex for the graph: the start of its newest revision's body (for
// the excerpt) and its tag names.
type GraphPage struct {
	ID       string
	Title    string
	FolderID string
	Body     string
	Tags     []string
}

// GraphLink is one stored [[link]] between two pages of a codex; ToPageID is "" when the target
// has no page.
type GraphLink struct {
	FromPageID string
	TargetText string
	ToPageID   string
}

// PageBody is a page with the body of its newest revision.
type PageBody struct {
	ID              string
	Title           string
	CurrentRevision int
	Body            string
}
