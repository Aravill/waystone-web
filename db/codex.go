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
	ErrWorldNotFound    = errors.New("world not found")
	ErrWorldNameTaken   = errors.New("a world with that name already exists")
	ErrWorldInUse       = errors.New("world still has games or pages")
	ErrFolderNotFound   = errors.New("folder not found")
	ErrFolderNameTaken  = errors.New("a folder with that name already exists here")
	ErrFolderCycle      = errors.New("a folder can't go inside itself")
	ErrNoFolders        = errors.New("the Unsorted folder has no folders")
	ErrParentNotFound   = errors.New("parent folder not found")
	ErrFolderTooDeep    = errors.New("folders can nest up to 10 levels")
	ErrPageNotFound     = errors.New("page not found")
	ErrNotAMap          = errors.New("page is not a map")
	ErrPinNotFound      = errors.New("pin not found")
	ErrPinEventNotFound = errors.New("pin event not found")
	ErrPinEventUndone   = errors.New("pin change already undone")
	ErrImageNotFound    = errors.New("image not found")
	ErrRevisionNotFound = errors.New("revision not found")
)

// NameConflict is another page in the same world that already has a title or alias.
type NameConflict struct {
	Name   string `json:"name"`
	PageID string `json:"page_id"`
	Title  string `json:"title"`
}

// NameTakenError is returned when a title or alias is already used inside the target world.
type NameTakenError struct{ Conflicts []NameConflict }

func (e *NameTakenError) Error() string {
	names := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		names[i] = c.Name
	}
	return fmt.Sprintf("already used in this world: %s", strings.Join(names, ", "))
}

func GetWorlds() ([]models.World, error)        { return GetStore().GetWorlds() }
func GetWorld(id string) (*models.World, error) { return GetStore().GetWorld(id) }
func CreateWorld(w models.World) error          { return GetStore().CreateWorld(w) }
func UpdateWorld(id, name, description string) error {
	return GetStore().UpdateWorld(id, name, description)
}
func DeleteWorld(id string) error { return GetStore().DeleteWorld(id) }
func SetCampaignWorld(campaignID, worldID string) error {
	return GetStore().SetCampaignWorld(campaignID, worldID)
}

func GetCodexPages() ([]models.PageListItem, error)         { return GetStore().GetCodexPages() }
func GetPage(id string) (*models.Page, error)               { return GetStore().GetPage(id) }
func GetPageLinks(fromID string) ([]models.PageLink, error) { return GetStore().GetPageLinks(fromID) }
func GetBacklinks(id string) ([]models.PageBacklink, error) { return GetStore().GetBacklinks(id) }
func GetWantedPages(worldID string) ([]models.WantedPage, error) {
	return GetStore().GetWantedPages(worldID)
}
func GetPageRevisions(id string) ([]models.PageRevision, error) {
	return GetStore().GetPageRevisions(id)
}
func GetPageRevision(id string, number int) (models.PageRevision, error) {
	return GetStore().GetPageRevision(id, number)
}

// GetRecentRevisions lists recent changes; worldID nil means every world.
func GetRecentRevisions(worldID *string, limit int) ([]models.RecentRevision, error) {
	return GetStore().GetRecentRevisions(worldID, limit)
}
func CreatePage(p models.Page, body string) error { return GetStore().CreatePage(p, body) }
func SavePageRevision(id string, base int, rev models.PageRevision) (models.PageSaveResult, error) {
	return GetStore().SavePageRevision(id, base, rev, PageKeepRevisions)
}
func RenamePage(id, newTitle, userID string) (int, error) {
	return GetStore().RenamePage(id, newTitle, userID, PageKeepRevisions)
}
func SetPageAliases(id string, aliases []string) error { return GetStore().SetPageAliases(id, aliases) }
func MovePage(id, worldID string) error                { return GetStore().MovePage(id, worldID) }

func GetFolders(worldID string) ([]models.Folder, error) { return GetStore().GetFolders(worldID) }
func GetFolder(id string) (*models.Folder, error)        { return GetStore().GetFolder(id) }
func CreateFolder(f models.Folder) error                 { return GetStore().CreateFolder(f) }
func RenameFolder(id, name string) error                 { return GetStore().RenameFolder(id, name) }
func MoveFolder(id, newParentID string) error            { return GetStore().MoveFolder(id, newParentID) }

// DeleteFolder re-parents the folder's sub-folders and pages one level up and returns how many moved.
func DeleteFolder(id string) (pages, folders int, err error) { return GetStore().DeleteFolder(id) }
func SetPageFolder(pageID, folderID string) error            { return GetStore().SetPageFolder(pageID, folderID) }

// DeletePage returns the map image files the caller should remove from disk.
func DeletePage(id string) ([]string, error) { return GetStore().DeletePage(id) }

func GetMapImages(pageID string) ([]models.MapImage, error) { return GetStore().GetMapImages(pageID) }
func AddMapImage(img models.MapImage) error                 { return GetStore().AddMapImage(img) }
func RestoreMapImage(pageID, imageID, userID string) (models.MapImage, error) {
	return GetStore().RestoreMapImage(pageID, imageID, userID)
}
func GetMapPins(pageID string) ([]models.MapPin, error)  { return GetStore().GetMapPins(pageID) }
func AddMapPin(pin models.MapPin) (models.MapPin, error) { return GetStore().AddMapPin(pin) }

// AddMapPinWithNewPage creates the page (in the map's world) and pins it, in one transaction.
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
