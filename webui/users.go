package webui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	IsAdmin      bool   `json:"is_admin"`
	CreatedAt    string `json:"created_at"`
}

type UserStore struct {
	users    map[string]*User
	filePath string
	mu       sync.RWMutex
}

var userStore *UserStore

func InitUserStore(filePath string) error {
	store := &UserStore{
		users:    make(map[string]*User),
		filePath: filePath,
	}

	if err := store.load(); err != nil {
		return err
	}

	if len(store.users) == 0 {
		password := generateRandomPassword()
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		store.users["admin"] = &User{
			Username:     "admin",
			PasswordHash: string(hash),
			IsAdmin:      true,
			CreatedAt:    "2026-01-01 00:00:00",
		}
		if err := store.save(); err != nil {
			return err
		}
		// Bootstrap credential is local-only and never goes to logs.
		if err := os.WriteFile(filepath.Join(filepath.Dir(filePath), ".admin-initial-password"), []byte(password+"\n"), 0600); err != nil {
			return err
		}

	}

	userStore = store
	return nil
}

func GetUserStore() *UserStore {
	return userStore
}

func (s *UserStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var users []*User
	if err := json.Unmarshal(data, &users); err != nil {
		return err
	}

	for _, u := range users {
		s.users[u.Username] = u
	}

	return nil
}

func (s *UserStore) save() error {
	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		copy := *u
		users = append(users, &copy)
	}

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return err
	}
	return os.Chmod(s.filePath, 0600)
}

func (s *UserStore) Authenticate(username, password string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[username]
	if !ok {
		return nil, false
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, false
	}

	copy := *user
	return &copy, true
}

func (s *UserStore) GetAllUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		copy := *u
		users = append(users, &copy)
	}
	return users
}

func (s *UserStore) GetUser(username string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[username]
	if !ok {
		return nil, false
	}
	copy := *user
	return &copy, true
}

func (s *UserStore) CreateUser(username, password string, isAdmin bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return errors.New("username already exists")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	s.users[username] = &User{
		Username:     username,
		PasswordHash: string(hash),
		IsAdmin:      isAdmin,
		CreatedAt:    "2026-01-01 00:00:00",
	}

	return s.save()
}

func (s *UserStore) DeleteUser(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if username == "admin" {
		return errors.New("cannot delete admin user")
	}

	if _, exists := s.users[username]; !exists {
		return errors.New("user not found")
	}

	delete(s.users, username)
	return s.save()
}

func (s *UserStore) ChangePassword(username, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[username]
	if !exists {
		return errors.New("user not found")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hash)
	return s.save()
}

func generateRandomPassword() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}
