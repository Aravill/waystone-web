package db

import (
	"fmt"
	"time"
	"waystone-web/auth"
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
	GetUserByUsername(username string) (*models.User, error)
	GetUserByID(id string) (*models.User, error)
	GetAllUsers() ([]models.User, error)
	SetUserNickname(id, nickname string) error
	SetUserAvatar(id string, data []byte, contentType string) error
	GetUserAvatar(id string) ([]byte, string, error)
	AnonymizeUser(id string) error
	SetUserBlocked(id string, blocked bool) error
	SetUserPassword(id, hash string, mustChange bool) error
	GetSessionsByMonth(campaignID string, year int, month int) ([]models.CampaignSession, error)
	GetSessionByID(sessionID string) (*models.CampaignSession, error)
	SaveSession(session models.CampaignSession) error
	GetSessionsByCampaignIDs(campaignIDs []string, fromDate string) ([]models.CampaignSession, error)
	GetCalendarToken(userID string) (string, error)
	SetCalendarToken(userID, token string) error
	GetUserIDByCalendarToken(token string) (string, error)
	UpsertSessionResponse(response models.SessionResponse) error
	GetSessionResponses(sessionID string) ([]models.SessionResponse, error)
	GetSessionResponsesBySessionIDs(sessionIDs []string) (map[string][]models.SessionResponse, error)
	DeleteSession(sessionID string) error
	CreateCampaignSignup(signup models.CampaignSignup) error
	GetCampaignSignup(campaignID, userID string) (*models.CampaignSignup, error)
	GetCampaignSignups(campaignID string) ([]models.CampaignSignup, error)
	GetCampaignSignupsByUser(userID string) (map[string]models.CampaignSignup, error)
	FollowCampaign(campaignID, userID string) error
	UnfollowCampaign(campaignID, userID string) error
	GetCampaignFollowerIDs(campaignID string) ([]string, error)
	GetFollowedCampaignIDs(userID string) (map[string]bool, error)
	CountPendingSignupsForDM(dmID string) (map[string]int, error)
	GetUnansweredSessions(userID, fromDate string) (map[string]models.UnansweredSessions, error)
	SetCampaignSignupStatus(campaignID, userID string, status models.SignupStatus) error
	WithdrawCampaignSignup(campaignID, userID string) (wasPlayer bool, err error)
	SetCampaignSignUpsOpen(campaignID string, open bool) error
	UpdateCampaignDetails(id, title, summary, description string, campaignType models.CampaignType, system, desiredPlayerCount string) error
	SetCampaignStatus(id string, status models.CampaignStatus) error
	DeleteCampaign(id string) error
	GetAllSystems() ([]models.System, error)
	CreateSystem(sys models.System) error
	RenameSystem(id, name string) error
	DeleteSystem(id string) error
	SetSystemLogo(id string, data []byte, contentType string) error
	GetSystemLogo(id string) ([]byte, string, error)
	CreateNotifications(notifications []models.Notification) error
	GetNotifications(userID string, limit int) ([]models.Notification, error)
	CountUnreadNotifications(userID string) (int, error)
	MarkNotificationRead(userID, id string) error
	MarkAllNotificationsRead(userID string) error
	Close() error
}

func Initialize() error {
	var err error
	sqlStore, err := NewSQLiteStore(config.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize sqlite database: %w", err)
	}

	store = sqlStore
	if err := seedIfEmpty(); err != nil {
		return err
	}
	return EnsureAdmin()
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
		systems, err := store.GetAllSystems()
		if err != nil {
			return err
		}
		if len(systems) == 0 {
			for _, sys := range config.InitialSystems {
				if err := store.CreateSystem(sys); err != nil {
					return fmt.Errorf("failed to seed system: %w", err)
				}
			}
		}
		for _, campaign := range config.InitialCampaigns {
			if err := store.SaveCampaign(campaign); err != nil {
				return fmt.Errorf("failed to seed campaign: %w", err)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		for _, seeded := range config.InitialSessions() {
			session := seeded.Session
			session.ID = GenerateUUID()
			session.CreatedAt, session.UpdatedAt = now, now
			if err := store.SaveSession(session); err != nil {
				return fmt.Errorf("failed to seed session: %w", err)
			}
			for userID, participation := range seeded.Responses {
				if err := store.UpsertSessionResponse(models.SessionResponse{
					ID:            GenerateUUID(),
					SessionID:     session.ID,
					UserID:        userID,
					Participation: participation,
					CreatedAt:     now,
					UpdatedAt:     now,
				}); err != nil {
					return fmt.Errorf("failed to seed session response: %w", err)
				}
			}
		}
	}

	// Seed users if empty
	if len(users) == 0 {
		for _, user := range config.InitialUsers {
			if password, ok := config.InitialPasswords[user.ID]; ok {
				hash, err := auth.HashPassword(password)
				if err != nil {
					return fmt.Errorf("failed to hash seed password: %w", err)
				}
				user.PasswordHash = hash
			}
			if err := SaveUser(user); err != nil {
				return fmt.Errorf("failed to seed user: %w", err)
			}
		}
	}

	return nil
}
