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
