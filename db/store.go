package db

import (
	"fmt"
	"waystone-web/config"
	"waystone-web/models"
)

var (
	store Store
)

type Store interface {
	GetAllCampaigns() ([]models.Campaign, error)
	SaveCampaign(campaign models.Campaign) error
	GetCampaignByID(id string) (*models.Campaign, error)
	SaveUser(user models.User) error
	GetUserByGoogleID(googleID string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByID(id string) (*models.User, error)
	GetAllUsers() ([]models.User, error)
	DeleteUser(id string) error
	Close() error
}

func Initialize() error {
	var err error
	sqlStore, err := NewSQLiteStore(config.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize sqlite database: %w", err)
	}

	store = sqlStore
	return seedIfEmpty()
}

func GetStore() Store {
	if store == nil {
		panic("database not initialized")
	}
	return store
}

func Close() error {
	if store != nil {
		return store.Close()
	}
	return nil
}

func seedIfEmpty() error {

	campaigns, err := store.GetAllCampaigns()
	if err != nil {
		return err
	}

	users, err := store.GetAllUsers()
	if err != nil {
		return err
	}

	// Only seed if database already has all seeded domains.
	if len(campaigns) > 0 && len(users) > 0 {
		return nil
	}

	// Seed campaigns if empty
	if len(campaigns) == 0 {
		for _, campaign := range config.InitialCampaigns {
			if err := store.SaveCampaign(campaign); err != nil {
				return fmt.Errorf("failed to seed campaign: %w", err)
			}
		}
	}

	// Seed users if empty
	if len(users) == 0 {
		for _, user := range config.InitialUsers {
			if err := store.SaveUser(user); err != nil {
				return fmt.Errorf("failed to seed user: %w", err)
			}
		}
	}

	return nil
}
