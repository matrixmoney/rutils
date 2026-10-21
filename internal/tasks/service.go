package tasks

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors returned by the service layer.
var (
	ErrEmptyText      = errors.New("task text is required")
	ErrTextTooLong    = errors.New("task text is too long (max 2000 characters)")
	ErrMissingDueDate = errors.New("due date is required (format: YYYY-MM-DD)")
	ErrInvalidDueDate = errors.New("due date must be in YYYY-MM-DD format")
)

const maxTextLength = 2000

// Service encapsulates the business rules for tasks.
type Service struct {
	store *Store
}

// NewService wires the store into the service.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// List returns all saved tasks, newest first.
func (s *Service) List() ([]Task, error) {
	return s.store.List()
}

// Create validates and persists a new task.
func (s *Service) Create(text, dueDate string) (*Task, error) {
	text, dueDate, err := validateFields(text, dueDate)
	if err != nil {
		return nil, err
	}
	return s.store.Create(text, dueDate)
}

// Update validates and persists an edit to an existing task.
func (s *Service) Update(id, text, dueDate string) (*Task, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrTaskNotFound
	}
	text, dueDate, err := validateFields(text, dueDate)
	if err != nil {
		return nil, err
	}
	return s.store.Update(id, text, dueDate)
}

// Delete removes a task by ID.
func (s *Service) Delete(id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrTaskNotFound
	}
	return s.store.Delete(id)
}

// validateFields trims whitespace and enforces the field rules.
// Returns the trimmed values on success.
func validateFields(text, dueDate string) (string, string, error) {
	text = strings.TrimSpace(text)
	dueDate = strings.TrimSpace(dueDate)

	if text == "" {
		return "", "", ErrEmptyText
	}
	if len(text) > maxTextLength {
		return "", "", ErrTextTooLong
	}
	if dueDate == "" {
		return "", "", ErrMissingDueDate
	}
	if _, err := time.Parse("2006-01-02", dueDate); err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrInvalidDueDate, err)
	}
	return text, dueDate, nil
}