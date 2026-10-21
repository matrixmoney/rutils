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
// already used by auth.Store and nifty.FileStorage.
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

// List returns every task, newest first (by CreatedAt).
func (s *Store) List() ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	// Return a copy so callers can't mutate our internal state.
	out := make([]Task, len(list.Tasks))
	copy(out, list.Tasks)

	// Newest first — matches the "phone notes" convention.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// Create inserts a new task and returns it with generated ID + timestamps.
func (s *Store) Create(text, dueDate string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	task := Task{
		ID:        generateID(),
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

// Update mutates an existing task identified by ID and returns the updated copy.
func (s *Store) Update(id, text, dueDate string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	for i := range list.Tasks {
		if list.Tasks[i].ID == id {
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

// Delete removes a task identified by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return err
	}

	for i := range list.Tasks {
		if list.Tasks[i].ID == id {
			list.Tasks = append(list.Tasks[:i], list.Tasks[i+1:]...)
			return s.saveUnlocked(list)
		}
	}
	return ErrTaskNotFound
}