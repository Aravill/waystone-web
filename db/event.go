package db

import (
	"waystone-web/models"
)

func GetAllEvents() ([]models.Event, error) {
	return GetStore().GetAllEvents()
}

func SaveEvent(event models.Event) error {
	return GetStore().SaveEvent(event)
}

func GetEventByID(id int) (*models.Event, error) {
	return GetStore().GetEventByID(id)
}

func GetNextEventID() int {
	events, err := GetAllEvents()
	if err != nil {
		return 1
	}

	maxID := 0
	for _, event := range events {
		if event.ID > maxID {
			maxID = event.ID
		}
	}
	return maxID + 1
}
