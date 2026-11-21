package db

import (
	"errors"
	"fmt"
	"strings"

	"waystone-web/models"
)

// Codex limits: revisions kept per page and pin events kept per map.
const (
	PageKeepRevisions = 200
	PinEventsKeep     = 200
)

var (
	ErrCodexNotFound     = errors.New("codex not found")
	ErrCodexNameTaken    = errors.New("a codex with that name already exists")
	ErrCodexInUse        = errors.New("codex still has pages")
	ErrFolderNotFound    = errors.New("folder not found")
	ErrFolderNameTaken   = errors.New("a folder with that name already exists here")
	ErrFolderCycle       = errors.New("a folder can't go inside itself")
	ErrParentNotFound    = errors.New("parent folder not found")
	ErrFolderTooDeep     = errors.New("folders can nest up to 10 levels")
	ErrPageNotFound      = errors.New("page not found")
	ErrNoMap             = errors.New("the page has no map")
	ErrPinNotFound       = errors.New("pin not found")
	ErrPinEventNotFound  = errors.New("pin event not found")
	ErrPinEventUndone    = errors.New("pin change already undone")
	ErrImageNotFound     = errors.New("image not found")
	ErrTagNotFound       = errors.New("tag not found")
	ErrTagNameTaken      = errors.New("a tag with that name already exists in this codex")
	ErrTemplateNotFound  = errors.New("template not found")
	ErrTemplateNameTaken = errors.New("a template with that name already exists in this codex")
	ErrRevisionNotFound  = errors.New("revision not found")
	ErrNotTimeline       = errors.New("the page is not a timeline")
	ErrEventNotFound     = errors.New("event not found")
	ErrLinkNotInCodex    = errors.New("the linked page is not in this codex")
)

// NameConflict is another page in the same codex that already has a title or alias.
type NameConflict struct {
	Name   string `json:"name"`
	PageID string `json:"page_id"`
	Title  string `json:"title"`
}

// NameTakenError is returned when a title or alias is already used inside the target codex.
type NameTakenError struct{ Conflicts []NameConflict }

func (e *NameTakenError) Error() string {
	names := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		names[i] = c.Name
	}
	return fmt.Sprintf("already used in this codex: %s", strings.Join(names, ", "))
}

func GetCodices() ([]models.Codex, error)       { return GetStore().GetCodices() }
func GetCodex(id string) (*models.Codex, error) { return GetStore().GetCodex(id) }
func CreateCodex(w models.Codex, creatorGetsEdit bool) error {
	return GetStore().CreateCodex(w, creatorGetsEdit)
}
func UpdateCodex(id, name, description string) error {
	return GetStore().UpdateCodex(id, name, description)
}
func DeleteCodex(id string) error { return GetStore().DeleteCodex(id) }
func SetCampaignCodex(campaignID, codexID string) error {
	return GetStore().SetCampaignCodex(campaignID, codexID)
}

func GetCodexPages() ([]models.PageListItem, error)         { return GetStore().GetCodexPages() }
func GetPage(id string) (*models.Page, error)               { return GetStore().GetPage(id) }
func GetPageLinks(fromID string) ([]models.PageLink, error) { return GetStore().GetPageLinks(fromID) }
func GetBacklinks(id string) ([]models.PageBacklink, error) { return GetStore().GetBacklinks(id) }
func GetWantedPages(codexID string) ([]models.WantedPage, error) {
	return GetStore().GetWantedPages(codexID)
}
func GetPageRevisions(id string) ([]models.PageRevision, error) {
	return GetStore().GetPageRevisions(id)
}
func GetPageRevision(id string, number int) (models.PageRevision, error) {
	return GetStore().GetPageRevision(id, number)
}

// GetRecentRevisions lists recent changes in the given codices; nil means every codex.
func GetRecentRevisions(codexIDs []string, limit int) ([]models.RecentRevision, error) {
	return GetStore().GetRecentRevisions(codexIDs, limit)
}
func CreatePage(p models.Page, body string) error { return GetStore().CreatePage(p, body) }
func SavePageRevision(id string, base int, rev models.PageRevision) (models.PageSaveResult, error) {
	return GetStore().SavePageRevision(id, base, rev, PageKeepRevisions)
}
func RenamePage(id, newTitle, userID string) (int, error) {
	return GetStore().RenamePage(id, newTitle, userID, PageKeepRevisions)
}
func SetPageAliases(id string, aliases []string) error { return GetStore().SetPageAliases(id, aliases) }
func MovePage(id, codexID string) error                { return GetStore().MovePage(id, codexID) }

func GetFolders(codexID string) ([]models.Folder, error) { return GetStore().GetFolders(codexID) }
func GetFolder(id string) (*models.Folder, error)        { return GetStore().GetFolder(id) }
func CreateFolder(f models.Folder) error                 { return GetStore().CreateFolder(f) }
func RenameFolder(id, name string) error                 { return GetStore().RenameFolder(id, name) }
func MoveFolder(id, newParentID string) error            { return GetStore().MoveFolder(id, newParentID) }

// DeleteFolder re-parents the folder's sub-folders and pages one level up and returns how many moved.
func DeleteFolder(id string) (pages, folders int, err error) { return GetStore().DeleteFolder(id) }
func SetPageFolder(pageID, folderID string) error            { return GetStore().SetPageFolder(pageID, folderID) }

// DeletePage returns the map image files the caller should remove from disk.
func DeletePage(id string) ([]string, error) { return GetStore().DeletePage(id) }

func GetCodexTags(codexID string) ([]models.CodexTag, error) { return GetStore().GetCodexTags(codexID) }
func GetCodexPageTags(codexID string) (map[string][]string, error) {
	return GetStore().GetCodexPageTags(codexID)
}
func GetPageTags(pageID string) ([]models.CodexTag, error) { return GetStore().GetPageTags(pageID) }
func GetTagCodexID(tagID string) (string, error)           { return GetStore().GetTagCodexID(tagID) }
func CreateCodexTag(codexID, name, userID string) (models.CodexTag, error) {
	return GetStore().CreateCodexTag(codexID, name, userID)
}
func RenameCodexTag(id, name string) error { return GetStore().RenameCodexTag(id, name) }

// DeleteCodexTag returns how many pages lost the tag.
func DeleteCodexTag(id string) (int, error) { return GetStore().DeleteCodexTag(id) }
func SetPageTags(pageID string, names []string, userID string) ([]models.CodexTag, error) {
	return GetStore().SetPageTags(pageID, names, userID)
}

// CreatePageWith is CreatePage plus tags, pictures and a revision note, in one transaction.
func CreatePageWith(p models.Page, body string, x models.NewPageExtras) error {
	return GetStore().CreatePageWith(p, body, x)
}

// CheckNewPage tells whether a page could be created there (see the store method) without writing.
func CheckNewPage(codexID, folderID, title string) error {
	return GetStore().CheckNewPage(codexID, folderID, title)
}

func GetCodexTemplates(codexID string) ([]models.CodexTemplate, error) {
	return GetStore().GetCodexTemplates(codexID)
}

// GetCodexTemplate returns nil, nil when there is no such template.
func GetCodexTemplate(id string) (*models.CodexTemplate, error) {
	return GetStore().GetCodexTemplate(id)
}
func CreateCodexTemplate(t models.CodexTemplate, tagNames []string) (models.CodexTemplate, error) {
	return GetStore().CreateCodexTemplate(t, tagNames)
}
func UpdateCodexTemplate(id, name, description, body string, tagNames []string, userID string) error {
	return GetStore().UpdateCodexTemplate(id, name, description, body, tagNames, userID)
}
func DeleteCodexTemplate(id string) error { return GetStore().DeleteCodexTemplate(id) }

// GetPageImages lists every picture ever uploaded to the page (removed ones included, oldest
// first) and the id of its cover.
func GetPageImages(pageID string) ([]models.PageImage, string, error) {
	return GetStore().GetPageImages(pageID)
}

// AddPageImage stores the picture row; img.Name is made unique within the page and returned.
func AddPageImage(img models.PageImage) (models.PageImage, error) {
	return GetStore().AddPageImage(img)
}
func RemovePageImage(pageID, imageID string) error {
	return GetStore().RemovePageImage(pageID, imageID)
}
func SetPageCover(pageID, imageID string) error { return GetStore().SetPageCover(pageID, imageID) }

func GetMapImages(pageID string) ([]models.MapImage, error) { return GetStore().GetMapImages(pageID) }
func AddMapImage(img models.MapImage) error                 { return GetStore().AddMapImage(img) }
func RestoreMapImage(pageID, imageID, userID string) (models.MapImage, error) {
	return GetStore().RestoreMapImage(pageID, imageID, userID)
}
func RemoveMap(pageID, userID string) (int, error)       { return GetStore().RemoveMap(pageID, userID) }
func GetMapPins(pageID string) ([]models.MapPin, error)  { return GetStore().GetMapPins(pageID) }
func AddMapPin(pin models.MapPin) (models.MapPin, error) { return GetStore().AddMapPin(pin) }

// AddMapPinWithNewPage creates the page (in the map's codex) and pins it, in one transaction.
func AddMapPinWithNewPage(page models.Page, pin models.MapPin) (models.MapPin, error) {
	return GetStore().AddMapPinWithNewPage(page, pin)
}
func MoveMapPin(mapID, pinID string, x, y float64, userID string) error {
	return GetStore().MoveMapPin(mapID, pinID, x, y, userID)
}
func DeleteMapPin(mapID, pinID, userID string) error {
	return GetStore().DeleteMapPin(mapID, pinID, userID)
}
func GetMapPinEvents(mapID string, limit int) ([]models.MapPinEvent, error) {
	return GetStore().GetMapPinEvents(mapID, limit)
}
func UndoMapPinEvent(mapID, eventID, userID string) error {
	return GetStore().UndoMapPinEvent(mapID, eventID, userID)
}

// GetCodexGraph returns a codex's pages and stored links, for the graph view.
func GetCodexGraph(codexID string) ([]models.GraphPage, []models.GraphLink, error) {
	return GetStore().GetCodexGraph(codexID)
}

// Timeline pages. Events link pages of the same codex; see sqlite_codex_timeline.go.
func GetTimelineEvents(pageID string) ([]models.TimelineEvent, error) {
	return GetStore().GetTimelineEvents(pageID)
}
func GetEventsLinkingTo(pageID string) ([]models.TimelineEvent, error) {
	return GetStore().GetEventsLinkingTo(pageID)
}
func CreateTimelineEvent(e models.TimelineEvent) (models.TimelineEvent, error) {
	return GetStore().CreateTimelineEvent(e)
}

// CreateTimelineEventWithPage creates the page (in the timeline's codex) and an event linking it, in one transaction.
func CreateTimelineEventWithPage(e models.TimelineEvent, p models.Page, body string, x models.NewPageExtras) (models.TimelineEvent, error) {
	return GetStore().CreateTimelineEventWithPage(e, p, body, x)
}
func UpdateTimelineEvent(e models.TimelineEvent) error { return GetStore().UpdateTimelineEvent(e) }
func DeleteTimelineEvent(pageID, eventID string) error {
	return GetStore().DeleteTimelineEvent(pageID, eventID)
}
func GetTimelineCalendar(pageID string) (models.TimelineCalendar, error) {
	return GetStore().GetTimelineCalendar(pageID)
}
func SetTimelineCalendar(pageID string, c models.TimelineCalendar, userID string) error {
	return GetStore().SetTimelineCalendar(pageID, c, userID)
}
