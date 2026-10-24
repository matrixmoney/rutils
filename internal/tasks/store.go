package tasks

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Sentinel errors returned by the store.
var (
	ErrTaskNotFound = errors.New("task not found")
)

// Store manages persistence of Task records in a JSON file.
//
// Concurrency-safe: read operations take RLock, write operations take
// Lock. Writes are atomic (write-to-temp + rename), mirroring the pattern
// already used by auth.Store.
//
// Ownership model: every task carries a UserID. Read/update/delete
// operations take the caller's userID and NEVER touch tasks owned by
// someone else. Cross-user access returns ErrTaskNotFound — the same
// error as a genuinely missing task — so clients cannot enumerate
// other users' task IDs.
type Store struct {
	filePath string
	mu       sync.RWMutex
}

// NewStore initializes a JSON file store at the specified path.
// If the file does not exist, an empty task list is written.
func NewStore(filePath string) (*Store, error) {
	if filePath == "" {
		filePath = "data/tasks.json"
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory for tasks store: %w", err)
	}

	s := &Store{filePath: filePath}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := s.saveUnlocked(TaskList{Tasks: []Task{}}); err != nil {
			return nil, fmt.Errorf("failed to initialize tasks store: %w", err)
		}
	}

	return s, nil
}

// FilePath returns the resolved path of the backing JSON file.
func (s *Store) FilePath() string { return s.filePath }

// loadUnlocked reads the task list from disk (caller must hold a lock).
func (s *Store) loadUnlocked() (TaskList, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return TaskList{Tasks: []Task{}}, nil
		}
		return TaskList{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return TaskList{Tasks: []Task{}}, nil
	}

	var list TaskList
	if err := json.Unmarshal(data, &list); err != nil {
		return TaskList{}, fmt.Errorf("failed to parse tasks JSON: %w", err)
	}
	if list.Tasks == nil {
		list.Tasks = []Task{}
	}
	return list, nil
}

// saveUnlocked atomically writes the task list to disk
// (caller must hold a write lock).
func (s *Store) saveUnlocked(list TaskList) error {
	if list.Tasks == nil {
		list.Tasks = []Task{}
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tasks JSON: %w", err)
	}

	dir := filepath.Dir(s.filePath)
	tmpFile, err := os.CreateTemp(dir, "tasks-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.filePath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to replace tasks file: %w", err)
	}
	return nil
}

// generateID creates a random hex string ID with the "tsk_" prefix.
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "tsk_" + hex.EncodeToString(b)
}

// GetByID returns the task with the given ID, but ONLY if it is owned
// by userID. Returns ErrTaskNotFound for missing tasks AND for tasks
// owned by another user (indistinguishable by design).
//
// Used by the handler before calling Update / Delete so it can capture
// the "old" state for the confirmation email.
func (s *Store) GetByID(userID, id string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("cannot look up task without an owner")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	for _, t := range list.Tasks {
		if t.ID == id && t.UserID == userID {
			found := t
			return &found, nil
		}
	}
	return nil, ErrTaskNotFound
}

// ListByUser returns only the tasks owned by userID, newest first.
//
// Tasks whose UserID is empty (legacy pre-scoping records) are
// deliberately excluded: they have no owner and therefore belong to
// nobody. The server logs how many such orphans it detected at startup
// so operators are not surprised by "missing" rows.
func (s *Store) ListByUser(userID string) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	// Copy matching tasks into a fresh slice so callers cannot mutate
	// our internal state and so the ordering is deterministic.
	filtered := make([]Task, 0)
	for _, t := range list.Tasks {
		if t.UserID == userID && userID != "" {
			filtered = append(filtered, t)
		}
	}

	// Newest first — matches the "phone notes" convention.
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	return filtered, nil
}

// CountOrphans returns the number of stored tasks whose UserID is empty.
// Used by cmd/server/main.go to log a one-line startup warning after a
// schema upgrade, so operators know legacy rows exist but are invisible.
func (s *Store) CountOrphans() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range list.Tasks {
		if strings.TrimSpace(t.UserID) == "" {
			n++
		}
	}
	return n, nil
}

// Create inserts a new task owned by userID and returns it with
// generated ID + timestamps. The caller (handler) MUST pass the
// authenticated user's ID — the store trusts it.
func (s *Store) Create(userID, text, dueDate string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("cannot create task without an owner")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	task := Task{
		ID:        generateID(),
		UserID:    userID,
		Text:      text,
		DueDate:   dueDate,
		CreatedAt: now,
		UpdatedAt: now,
	}
	list.Tasks = append(list.Tasks, task)

	if err := s.saveUnlocked(list); err != nil {
		return nil, err
	}
	return &task, nil
}

// Update mutates an existing task identified by ID, but ONLY if it is
// owned by userID. Cross-user updates return ErrTaskNotFound so the
// client cannot distinguish "not found" from "not yours".
func (s *Store) Update(userID, id, text, dueDate string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("cannot update task without an owner")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	for i := range list.Tasks {
		if list.Tasks[i].ID == id && list.Tasks[i].UserID == userID {
			list.Tasks[i].Text = text
			list.Tasks[i].DueDate = dueDate
			list.Tasks[i].UpdatedAt = time.Now().UTC()

			if err := s.saveUnlocked(list); err != nil {
				return nil, err
			}
			updated := list.Tasks[i]
			return &updated, nil
		}
	}

	return nil, ErrTaskNotFound
}

// Delete removes a task identified by ID, but ONLY if it is owned by
// userID. Cross-user deletes return ErrTaskNotFound.
//
// CHANGED: on success the function now returns the task as it looked
// immediately BEFORE it was removed from the file. The API handler uses
// this snapshot to populate the "Task Deleted" email body — the task is
// already gone from disk by the time the caller dispatches the email,
// so this is the only way to include its text and due date.
//
// The returned *Task is a copy of the internal record; the caller cannot
// mutate store state through it.
func (s *Store) Delete(userID, id string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("cannot delete task without an owner")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	for i := range list.Tasks {
		if list.Tasks[i].ID == id && list.Tasks[i].UserID == userID {
			// Snapshot BEFORE removal so the handler can email it.
			deleted := list.Tasks[i]

			list.Tasks = append(list.Tasks[:i], list.Tasks[i+1:]...)
			if err := s.saveUnlocked(list); err != nil {
				return nil, err
			}
			return &deleted, nil
		}
	}
	return nil, ErrTaskNotFound
}