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
	SaveSessionAttendance(sessionID string, guests []string, dmAbsent bool, attendeeIDs []string) error
	DeleteManualSession(sessionID string) error
	GetSessionsByCampaignIDs(campaignIDs []string, fromDate string) ([]models.CampaignSession, error)
	GetAcceptedConfirmedSessions(userID string) ([]models.CampaignSession, error)
	GetUnannouncedConfirmedSessions(fromDate, toDate string) ([]models.CampaignSession, error)
	GetUnremindedConfirmedSessions(fromDate, toDate string) ([]models.CampaignSession, error)
	GetDiscordSettings() (models.DiscordSettings, error)
	SaveDiscordSettings(s models.DiscordSettings) (cleared int, err error)
	SetCampaignDiscordChannel(campaignID, channelID, channelName string) error
	ClearLostDiscordChannel(campaignID, channelID string) (bool, error)
	GetDiscordLink(userID string) (*models.DiscordLink, error)
	SaveDiscordLink(userID, discordUserID, username string) error
	GetDiscordLinksForUsers(userIDs []string) (map[string]models.DiscordLink, error)
	SetDiscordToggles(userID string, sessions, signups, characters bool) error
	DeleteDiscordLink(userID string) error
	SetDiscordDMChannel(userID, channelID string) error
	SetDiscordDMError(userID, msg string) error
	EnqueueDiscord(rows []models.DiscordOutboxRow) error
	NextDiscordOutbox(now string) (*models.DiscordOutboxRow, error)
	DeleteDiscordOutbox(id int64) error
	UpdateDiscordOutbox(id int64, status string, attempts int, nextAttemptAt, lastError string) error
	PruneDiscordOutbox(failedBefore string) error
	GetDiscordOutboxSummary(limit int) (queued int, failures []models.DiscordOutboxRow, err error)
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
	GetUnansweredSessions(userID, fromDate string) ([]models.CampaignSession, error)
	CreatePollSessions(sessions []models.CampaignSession) error
	GetPollSessions(campaignID, pollID string) ([]models.CampaignSession, error)
	DeletePoll(campaignID, pollID string) (int, error)
	ConfirmSession(sessionID string) error
	GetPollReminderCandidates(fromDate, toDate string) ([]models.CampaignSession, error)
	GetPollReminderUserIDs(pollIDs []string) (map[string]map[string]bool, error)
	GetPollSchedule(campaignID string) (*models.PollSchedule, error)
	GetPollSchedules() ([]models.PollSchedule, error)
	SavePollSchedule(schedule models.PollSchedule) error
	DeletePollSchedule(campaignID string) error
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
	GetNotifications(userID string, unreadOnly bool, limit, offset int) ([]models.Notification, error)
	CountUnreadNotifications(userID string) (int, error)
	MarkNotificationRead(userID, id string) error
	MarkAllNotificationsRead(userID string) error
	// SaveJournalRevision writes the legacy journal tables and is only kept for the seed, which
	// the codex migration then moves into pages.
	SaveJournalRevision(campaignID, sessionID, entryID string, baseRevision int, rev models.JournalRevision, keepRevisions int) (models.JournalSaveResult, error)
	GetJournalPages(campaignID string) (map[string]models.JournalPage, error)
	GetJournalPageID(sessionID string) (string, error)
	CountLegacyJournal(campaignID string) (int, error)
	EnsureJournalFolder(campaignID, codexID, userID string) (folderID, template string, err error)
	CreateJournalPage(campaignID string, p models.Page, base, body string, x models.NewPageExtras) (pageID string, created bool, err error)
	RenameJournalFolder(campaignID, oldTitle, newTitle string) error
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
	GetCodices() ([]models.Codex, error)
	GetCodex(id string) (*models.Codex, error)
	CreateCodex(w models.Codex, creatorGetsEdit bool) error
	UpdateCodex(id, name, description string) error
	DeleteCodex(id string) error
	SetCampaignCodex(campaignID, codexID string) error
	GetUserRights(userID string) ([]string, error)
	GetAllUserRights() (map[string][]string, error)
	SetUserRight(userID, action string, on bool) error
	GetCodexAccessForUser(userID string) (map[string]string, error)
	GetAllCodexAccess() ([]models.CodexAccess, error)
	SetCodexAccess(codexID, userID, level string) error
	GetPageCodexID(pageID string) (string, error)
	GetImageCodexID(file string) (string, error)
	GetCodexPages() ([]models.PageListItem, error)
	GetPage(id string) (*models.Page, error)
	GetPageLinks(fromID string) ([]models.PageLink, error)
	GetBacklinks(id string) ([]models.PageBacklink, error)
	GetCodexBodies(codexID string) ([]models.PageBody, error)
	CountPagesByAuthor(userID string) (map[string]int, error)
	GetWantedPages(codexID string) ([]models.WantedPage, error)
	GetPageRevisions(id string) ([]models.PageRevision, error)
	GetPageRevision(id string, number int) (models.PageRevision, error)
	GetRecentRevisions(codexIDs []string, limit int) ([]models.RecentRevision, error)
	CreatePage(p models.Page, body string) error
	SavePageRevision(id string, base int, rev models.PageRevision, keep int) (models.PageSaveResult, error)
	RenamePage(id, newTitle, userID string, keep int) (int, error)
	SetPageAliases(id string, aliases []string) error
	MovePage(id, codexID string) error
	GetFolders(codexID string) ([]models.Folder, error)
	GetFolder(id string) (*models.Folder, error)
	CreateFolder(f models.Folder) error
	RenameFolder(id, name string) error
	MoveFolder(id, newParentID string) error
	SetFolderTemplate(id, templateID string) error
	DeleteFolder(id string) (pages, folders int, err error)
	SetPageFolder(pageID, folderID string) error
	DeletePage(id string) (images, attachments []string, err error)
	GetCodexTags(codexID string) ([]models.CodexTag, error)
	GetCodexPageTags(codexID string) (map[string][]string, error)
	GetCodexGraph(codexID string) ([]models.GraphPage, []models.GraphLink, error)
	GetPageTags(pageID string) ([]models.CodexTag, error)
	GetTagCodexID(tagID string) (string, error)
	CreateCodexTag(codexID, name, userID string) (models.CodexTag, error)
	RenameCodexTag(id, name string) error
	DeleteCodexTag(id string) (int, error)
	SetPageTags(pageID string, names []string, userID string) ([]models.CodexTag, error)
	CreatePageWith(p models.Page, body string, x models.NewPageExtras) error
	GetCodexNotes(codexID, userID string) ([]models.CodexNote, error)
	CountCodexNotes(codexID, userID string) (int, error)
	CreateCodexNote(n models.CodexNote, max int) error
	GetCodexNote(id string) (*models.CodexNote, error)
	UpdateCodexNote(id, body string) error
	DeleteCodexNote(id string) error
	AppendNoteToPage(noteID, userID, pageID, addition string, maxRunes, keep int) (models.PageRevision, error)
	CheckNewPage(codexID, folderID, title string) error
	GetCodexTemplates(codexID string) ([]models.CodexTemplate, error)
	GetCodexTemplate(id string) (*models.CodexTemplate, error)
	CreateCodexTemplate(t models.CodexTemplate, tagNames []string) (models.CodexTemplate, error)
	UpdateCodexTemplate(id, name, description, body string, tagNames []string, userID string) error
	DeleteCodexTemplate(id string) error
	GetPageImages(pageID string) ([]models.PageImage, string, error)
	AddPageImage(img models.PageImage) (models.PageImage, error)
	RemovePageImage(pageID, imageID string) error
	SetPageCover(pageID, imageID string) error
	GetPageAttachments(pageID string) ([]models.PageAttachment, error)
	GetCodexAttachments(codexID string) ([]models.CodexAttachment, error)
	GetAttachment(id string) (models.PageAttachment, string, error)
	AddPageAttachment(a models.PageAttachment) (models.PageAttachment, error)
	ReplacePageAttachment(pageID, id, file string, size int64, userID string) (models.PageAttachment, string, error)
	RemovePageAttachment(pageID, id string) (string, error)
	GetMapImages(pageID string) ([]models.MapImage, error)
	AddMapImage(img models.MapImage) error
	RestoreMapImage(pageID, imageID, userID string) (models.MapImage, error)
	RemoveMap(pageID, userID string) (int, error)
	GetTimelineEvents(pageID string) ([]models.TimelineEvent, error)
	GetEventsLinkingTo(pageID string) ([]models.TimelineEvent, error)
	CreateTimelineEvent(e models.TimelineEvent) (models.TimelineEvent, error)
	CreateTimelineEventWithPage(e models.TimelineEvent, p models.Page, body string, x models.NewPageExtras) (models.TimelineEvent, error)
	UpdateTimelineEvent(e models.TimelineEvent) error
	DeleteTimelineEvent(pageID, eventID string) error
	GetTimelineCalendar(pageID string) (models.TimelineCalendar, error)
	SetTimelineCalendar(pageID string, c models.TimelineCalendar, userID string) error
	GetMapPins(pageID string) ([]models.MapPin, error)
	AddMapPin(pin models.MapPin) (models.MapPin, error)
	AddMapPinWithNewPage(page models.Page, pin models.MapPin) (models.MapPin, error)
	MoveMapPin(mapID, pinID string, x, y float64, userID string) error
	DeleteMapPin(mapID, pinID, userID string) error
	GetMapPinEvents(mapID string, limit int) ([]models.MapPinEvent, error)
	UndoMapPinEvent(mapID, eventID, userID string) error
	Backup(path string) error
	Close() error
}

func Initialize() error {
	var err error
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = config.DBPath
	}
	dbPath = path
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

	// The demo codex links to the seeded game, so it is added with the characters.
	if len(campaigns) == 0 && len(users) == 0 {
		if err := seedCodex(); err != nil {
			return err
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
