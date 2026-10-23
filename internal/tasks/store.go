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

var (
	ErrTaskNotFound = errors.New("task not found")
)

type Store struct {
	filePath string
	mu       sync.RWMutex
}

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

func (s *Store) FilePath() string { return s.filePath }

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

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "tsk_" + hex.EncodeToString(b)
}

// ListByUser returns only the tasks owned by userID, newest first.
func (s *Store) ListByUser(userID string) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	filtered := make([]Task, 0)
	for _, t := range list.Tasks {
		if t.UserID == userID && userID != "" {
			filtered = append(filtered, t)
		}
	}

	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	return filtered, nil
}

// CountOrphans returns the number of stored tasks with no owner.
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

// Create inserts a new task owned by userID.
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

// Update mutates an existing task, but ONLY if it is owned by userID.
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

// Delete removes a task, but ONLY if it is owned by userID.
func (s *Store) Delete(userID, id string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("cannot delete task without an owner")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return err
	}

	for i := range list.Tasks {
		if list.Tasks[i].ID == id && list.Tasks[i].UserID == userID {
			list.Tasks = append(list.Tasks[:i], list.Tasks[i+1:]...)
			return s.saveUnlocked(list)
		}
	}
	return ErrTaskNotFound
}