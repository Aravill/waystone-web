package db

import (
	"encoding/json"
	"fmt"
	"waystone-web/config"
	"waystone-web/models"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	store *LevelDBStore
)

type Store interface {
	GetAllEvents() ([]models.Event, error)
	SaveEvent(event models.Event) error
	GetEventByID(id int) (*models.Event, error)
	GetAllCampaigns() ([]models.Campaign, error)
	SaveCampaign(campaign models.Campaign) error
	GetCampaignByID(id int) (*models.Campaign, error)
	SaveSignup(signup models.Signup) error
	GetAllSignups() ([]models.Signup, error)
	SaveUser(user models.User) error
	GetUserByGoogleID(googleID string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetAllUsers() ([]models.User, error)
	Close() error
}

type LevelDBStore struct {
	db    *leveldb.DB
	mutex sync.RWMutex
}

func Initialize() error {
	var err error
	db, err := leveldb.OpenFile(config.DBPath, nil)
	if err != nil {
		return fmt.Errorf("failed to open leveldb: %w", err)
	}

	store = &LevelDBStore{db: db}
	return seedIfEmpty()
}

func GetStore() Store {
	if store == nil {
		panic("database not initialized")
	}
	return store
}

func Close() error {
	if store != nil && store.db != nil {
		return store.db.Close()
	}
	return nil
}

func (s *LevelDBStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func seedIfEmpty() error {
	events, err := store.GetAllEvents()
	if err != nil {
		return err
	}

	users, err := store.GetAllUsers()
	if err != nil {
		return err
	}

	// Only seed if database is empty (no events and no users)
	if len(events) > 0 && len(users) > 0 {
		return nil
	}

	// Seed events if empty
	if len(events) == 0 {
		for _, event := range config.InitialEvents {
			if err := store.SaveEvent(event); err != nil {
				return fmt.Errorf("failed to seed event: %w", err)
			}
		}
	}

	// Seed users if empty
	if len(users) == 0 {
		for _, user := range config.InitialUsers {
			if err := SaveUser(user); err != nil {
				return fmt.Errorf("failed to seed user: %w", err)
			}
		}
	}

	return nil
}

func (s *LevelDBStore) GetAllEvents() ([]models.Event, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var events []models.Event
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		key := string(iter.Key())
		if len(key) > 6 && key[:6] == "event:" {
			var event models.Event
			err := json.Unmarshal(iter.Value(), &event)
			if err == nil {
				events = append(events, event)
			}
		}
	}
	return events, nil
}

func (s *LevelDBStore) SaveEvent(event models.Event) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("event:%d", event.ID)
	return s.db.Put([]byte(key), data, nil)
}

func (s *LevelDBStore) GetEventByID(id int) (*models.Event, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	key := fmt.Sprintf("event:%d", id)
	data, err := s.db.Get([]byte(key), nil)
	if err != nil {
		return nil, err
	}

	var event models.Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *LevelDBStore) SaveSignup(signup models.Signup) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	data, err := json.Marshal(signup)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("signup:%d", time.Now().UnixNano())
	return s.db.Put([]byte(key), data, nil)
}

func (s *LevelDBStore) GetAllSignups() ([]models.Signup, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var signups []models.Signup
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		key := string(iter.Key())
		if len(key) > 7 && key[:7] == "signup:" {
			var signup models.Signup
			err := json.Unmarshal(iter.Value(), &signup)
			if err == nil {
				signups = append(signups, signup)
			}
		}
	}
	return signups, nil
}

// deleteUserEmailIndex removes the email index entry for a user
// Must be called while holding the write lock
func (s *LevelDBStore) deleteUserEmailIndex(email string) error {
	if email == "" {
		return nil
	}
	indexKey := fmt.Sprintf("email:%s", email)
	return s.db.Delete([]byte(indexKey), nil)
}

func (s *LevelDBStore) SaveUser(user models.User) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// If user exists and email has changed, delete old index
	if user.ID != "" {
		existingData, err := s.db.Get([]byte(fmt.Sprintf("user:%s", user.ID)), nil)
		if err == nil && existingData != nil {
			var existing models.User
			if err := json.Unmarshal(existingData, &existing); err == nil {
				if existing.Email != "" && existing.Email != user.Email {
					s.deleteUserEmailIndex(existing.Email)
				}
			}
		}
	}

	// Marshal user data
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	// Write user record
	userKey := fmt.Sprintf("user:%s", user.ID)
	if err := s.db.Put([]byte(userKey), data, nil); err != nil {
		return err
	}

	// Write email index if email is not empty
	if user.Email != "" {
		indexKey := fmt.Sprintf("email:%s", user.Email)
		if err := s.db.Put([]byte(indexKey), []byte(user.ID), nil); err != nil {
			return err
		}
	}

	return nil
}

func (s *LevelDBStore) GetUserByGoogleID(googleID string) (*models.User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	key := fmt.Sprintf("user:%s", googleID)
	data, err := s.db.Get([]byte(key), nil)
	if err != nil {
		return nil, err
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *LevelDBStore) GetAllUsers() ([]models.User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var users []models.User
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		key := string(iter.Key())
		if len(key) > 5 && key[:5] == "user:" {
			var user models.User
			err := json.Unmarshal(iter.Value(), &user)
			if err == nil {
				users = append(users, user)
			}
		}
	}
	return users, nil
}

func (s *LevelDBStore) GetUserByEmail(email string) (*models.User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if email == "" {
		return nil, nil
	}

	// Look up userId from email index
	indexKey := fmt.Sprintf("email:%s", email)
	userIDBytes, err := s.db.Get([]byte(indexKey), nil)
	if err != nil {
		// Key not found - return nil (not an error)
		return nil, nil
	}

	userID := string(userIDBytes)
	if userID == "" {
		return nil, nil
	}

	// Fetch full user from user:{userId}
	userKey := fmt.Sprintf("user:%s", userID)
	data, err := s.db.Get([]byte(userKey), nil)
	if err != nil {
		return nil, nil
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, nil
	}

	return &user, nil
}
