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
//
// Every method takes the caller's userID as its first argument. The
// service never assumes identity — it is the handler's job to extract
// it from the JWT. This keeps the service trivially testable and makes
// the ownership boundary explicit at every call site.
//
// The service deliberately knows nothing about email. Persistence and
// validation live here; notification is the handler's job (it has the
// authenticated user's email + name from the JWT claims, and it owns
// the goroutine-timeout wrapper that guards the SMTP call).
type Service struct {
	store *Store
}

// NewService wires the store into the service.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// GetByID returns a single task owned by userID.
//
// Used by the API handler to capture the "before" state of a task
// ahead of an Update or Delete, so the confirmation email can show
// what actually changed (update) or what was removed (delete).
//
// Cross-user lookups return ErrTaskNotFound — same as a genuinely
// missing task, so clients cannot enumerate other users' IDs.
func (s *Service) GetByID(userID, id string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}
	if strings.TrimSpace(id) == "" {
		return nil, ErrTaskNotFound
	}
	return s.store.GetByID(userID, id)
}

// List returns all tasks owned by userID, newest first.
func (s *Service) List(userID string) ([]Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}
	return s.store.ListByUser(userID)
}

// Create validates and persists a new task owned by userID.
func (s *Service) Create(userID, text, dueDate string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}
	text, dueDate, err := validateFields(text, dueDate)
	if err != nil {
		return nil, err
	}
	return s.store.Create(userID, text, dueDate)
}

// Update validates and persists an edit to an existing task, but only
// if the task is owned by userID.
func (s *Service) Update(userID, id, text, dueDate string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}
	if strings.TrimSpace(id) == "" {
		return nil, ErrTaskNotFound
	}
	text, dueDate, err := validateFields(text, dueDate)
	if err != nil {
		return nil, err
	}
	return s.store.Update(userID, id, text, dueDate)
}

// Delete removes a task by ID, but only if it is owned by userID.
//
// CHANGED: on success the function now returns the task as it looked
// immediately BEFORE it was removed. The handler uses this snapshot
// to populate the "Task Deleted" email — by the time the email is
// dispatched, the row is already gone from disk, so this is the only
// way to include its text and due date.
func (s *Service) Delete(userID, id string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}
	if strings.TrimSpace(id) == "" {
		return nil, ErrTaskNotFound
	}
	return s.store.Delete(userID, id)
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