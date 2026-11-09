package models

// UnsortedWorldID is how the API names the shared "Unsorted" folder; the database stores "".
const UnsortedWorldID = "unsorted"

const (
	PageLore = "lore"
	PageMap  = "map"
)

// World is a folder of Codex pages. Games point at a world with campaigns.world_id (a label only).
type World struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// PageContributor is one person's credit on a page. It outlives pruning of old revisions.
type PageContributor struct {
	UserID        string
	Revisions     int
	FirstRevision int
}

// Page is a Codex page with its newest revision. WorldID "" is the Unsorted folder.
type Page struct {
	ID              string
	Kind            string
	WorldID         string
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
	ID      string
	Kind    string
	WorldID string
	Title   string
	Aliases []string
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
	WorldID   string
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

// PageBacklink is a page (via a link) or a map (via a pin) that points at a page.
type PageBacklink struct {
	PageID     string
	Title      string
	Kind       string
	TargetText string
	PinID      string // set for pins
}

// WantedPage is a title that links or pins point at but no page has yet.
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

// MapPin sits at x,y (fractions of the image) and points at a page by title text.
type MapPin struct {
	ID         string  `json:"id"`
	MapPageID  string  `json:"map_page_id"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	TargetText string  `json:"target_text"`
	ToPageID   string  `json:"to_page_id"`
	CreatedBy  string  `json:"created_by"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// MapPinEvent logs a pin change. Before and After are JSON MapPin snapshots ("" when absent).
type MapPinEvent struct {
	ID        string
	MapPageID string
	PinID     string
	UserID    string
	Action    string // add | move | remove
	Before    string
	After     string
	UndoneBy  string
	CreatedAt string
}
