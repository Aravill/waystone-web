package db

import (
	"fmt"
	"log"
	"os"
	"strconv"
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
	SetUserProfile(id string, name, nickname *string) error
	CompleteProfileSetup(id string) error
	SetUserAvatar(id string, data []byte, contentType string) error
	GetUserAvatar(id string) ([]byte, string, error)
	AnonymizeUser(id string) error
	SetUserBlocked(id string, blocked bool) error
	SetUserPassword(id, hash string) error
	UpdateUserRoles(id string, roles []string) error
	GetSessionsByMonth(campaignID string, year int, month int) ([]models.CampaignSession, error)
	GetSessionByID(sessionID string) (*models.CampaignSession, error)
	SaveSession(session models.CampaignSession) error
	SaveManualSession(session models.CampaignSession, attendeeIDs []string) error
	DeleteManualSession(sessionID string) error
	GetSessionsByCampaignIDs(campaignIDs []string, fromDate string) ([]models.CampaignSession, error)
	GetAcceptedConfirmedSessions(userID string) ([]models.CampaignSession, error)
	GetUnannouncedConfirmedSessions(fromDate, toDate string) ([]models.CampaignSession, error)
	GetCalendarToken(userID string) (string, error)
	SetCalendarToken(userID, token string) error
	GetUserIDByCalendarToken(token string) (string, error)
	IssuePasswordInvite(userID, tokenHash string, expiresAt time.Time) error
	GetPasswordInvite(tokenHash string) (userID string, expiresAt time.Time, err error)
	DeletePasswordInvite(userID string) error
	RedeemPasswordInvite(tokenHash, passwordHash string) (userID string, err error)
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
	UpdateCampaignDetails(id, title, summary, description string, campaignType models.CampaignType, system, desiredPlayerCount string, systemIDChanges bool) error
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
	GetJournalSummaries(campaignID string) (map[string]models.JournalSummary, error)
	GetJournalEntry(sessionID string) (*models.JournalEntry, error)
	GetJournalRevisions(sessionID string) (entryID string, revs []models.JournalRevision, err error)
	SaveJournalRevision(campaignID, sessionID, entryID string, baseRevision int, rev models.JournalRevision, keepRevisions int) (models.JournalSaveResult, error)
	DeleteJournalEntry(sessionID string) error
	CreateCharacter(c models.Character) error
	GetCharacter(id string) (*models.Character, error)
	GetCharactersByOwner(ownerID string) ([]models.Character, error)
	UpdateCharacter(c models.Character) error
	SetCharacterStatus(id string, status models.CharacterStatus) error
	DeleteCharacter(id string) error
	GetAssignmentsByCharacterIDs(ids []string) (map[string][]models.CharacterAssignment, error)
	GetCampaignAssignments(campaignID string) ([]models.CharacterAssignment, error)
	GetActiveAssignmentsByUser(userID string) (map[string]models.CharacterAssignment, error)
	AssignCharacter(campaignID, userID, characterID, systemID, at string) (bool, error)
	EndCharacterAssignment(campaignID, userID, at string) (*models.CharacterAssignment, error)
	CountCharactersBySystem(systemID string) (int, error)
	GetWorlds() ([]models.World, error)
	GetWorld(id string) (*models.World, error)
	CreateWorld(w models.World) error
	UpdateWorld(id, name, description string) error
	DeleteWorld(id string) error
	SetCampaignWorld(campaignID, worldID string) error
	GetCodexPages() ([]models.PageListItem, error)
	GetPage(id string) (*models.Page, error)
	GetPageLinks(fromID string) ([]models.PageLink, error)
	GetBacklinks(id string) ([]models.PageBacklink, error)
	GetWantedPages(worldID string) ([]models.WantedPage, error)
	GetPageRevisions(id string) ([]models.PageRevision, error)
	GetRecentRevisions(worldID *string, limit int) ([]models.RecentRevision, error)
	CreatePage(p models.Page, body string) error
	SavePageRevision(id string, base int, rev models.PageRevision, keep int) (models.PageSaveResult, error)
	RenamePage(id, newTitle, userID string, keep int) (int, error)
	SetPageAliases(id string, aliases []string) error
	MovePage(id, worldID string) error
	DeletePage(id string) ([]string, error)
	Backup(path string) error
	Close() error
}

func Initialize() error {
	var err error
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = config.DBPath
	}
	sqlStore, err := NewSQLiteStore(path)
	if err != nil {
		return fmt.Errorf("failed to initialize sqlite database: %w", err)
	}

	store = sqlStore
	if err := seedIfEmpty(); err != nil {
		return err
	}
	return EnsureAdmin()
}

func Backup(path string) error {
	return GetStore().Backup(path)
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

// seedDemoData reports whether SEED_DEMO_DATA asks for the demo users,
// campaigns and sessions (local development only: the demo users share a
// password that is public in this repo).
func seedDemoData() bool {
	v := os.Getenv("SEED_DEMO_DATA")
	if v == "" {
		return false
	}
	on, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("WARNING: SEED_DEMO_DATA=%q is not a boolean - demo data disabled", v)
		return false
	}
	return on
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

	// Reference data: the game systems list, seeded only into a brand-new
	// database (this runs before EnsureAdmin, so a fresh DB has no users) so
	// systems an admin deleted don't come back on restart.
	if len(campaigns) == 0 && len(users) == 0 {
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
	}

	if !seedDemoData() {
		return nil
	}
	log.Println("WARNING: SEED_DEMO_DATA is on - demo accounts (testy, roy, adam, simon) with a publicly known password may exist. Never enable it on a public instance.")

	// Only seed if database already has all seeded domains.
	if len(campaigns) > 0 && len(users) > 0 {
		return nil
	}

	// Seed demo campaigns if empty
	if len(campaigns) == 0 {
		for _, campaign := range config.InitialCampaigns {
			if err := store.SaveCampaign(campaign); err != nil {
				return fmt.Errorf("failed to seed campaign: %w", err)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		// Sign-ups point at seed users, so only add them when those users are seeded too.
		for _, signup := range config.InitialSignups {
			if len(users) > 0 {
				break
			}
			signup.CreatedAt, signup.UpdatedAt = now, now
			if err := store.CreateCampaignSignup(signup); err != nil {
				return fmt.Errorf("failed to seed signup: %w", err)
			}
		}
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
			entryID, base := "", 0
			for _, rev := range seeded.Journal {
				result, err := store.SaveJournalRevision(session.CampaignID, session.ID, entryID, base, models.JournalRevision{
					AuthorID: rev.AuthorID, Name: rev.Name, Body: rev.Body, Note: rev.Note,
				}, JournalKeepRevisions)
				if err != nil {
					return fmt.Errorf("failed to seed journal entry: %w", err)
				}
				entryID, base = result.EntryID, result.Revision.Number
			}
		}
	}

	// Characters and assignments point at seed users, so only add them when those users are seeded too.
	if len(campaigns) == 0 && len(users) == 0 {
		at := func(days int) string { return time.Now().AddDate(0, 0, days).UTC().Format(time.RFC3339) }
		created := at(-100)
		systemOf := map[string]string{}
		for _, c := range config.InitialCharacters {
			systemOf[c.ID] = c.SystemID
			c.CreatedAt, c.UpdatedAt = created, created
			if err := store.CreateCharacter(c); err != nil {
				return fmt.Errorf("failed to seed character: %w", err)
			}
		}
		for _, a := range config.InitialCharacterAssignments {
			if _, err := store.AssignCharacter(a.CampaignID, a.UserID, a.CharacterID, systemOf[a.CharacterID], at(a.StartDays)); err != nil {
				return fmt.Errorf("failed to seed character assignment: %w", err)
			}
			if a.EndDays != 0 {
				if _, err := store.EndCharacterAssignment(a.CampaignID, a.UserID, at(a.EndDays)); err != nil {
					return fmt.Errorf("failed to end seeded character assignment: %w", err)
				}
			}
		}
	}

	// Seed demo users if empty
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
