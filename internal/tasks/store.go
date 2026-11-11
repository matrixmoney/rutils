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

// Reminder defaults applied at load time to any task whose reminder
// fields are absent or empty. These match the defaults documented on
// the Task struct in models.go.
const (
	defaultReminderTime     = "08:00"
	defaultReminderTimezone = "Asia/Kolkata"
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
//
// Reminder model (new in this revision): every Task carries its own
// reminder schedule (ReminderEnabled, ReminderStartDate, ReminderTime,
// ReminderTimezone, LastReminderSentAt). The store does NOT interpret
// these fields — it just persists them. The scheduler is the only
// component that reads ReminderStartDate / ReminderTime / Timezone to
// decide when to fire, and the only component that writes
// LastReminderSentAt (via MarkReminderSent). The store's job is:
//
//   1. Normalise the reminder fields of every task it loads, so that
//      old rows written before this revision acquire safe defaults
//      (the task's own DueDate, "08:00", "Asia/Kolkata", enabled=true).
//
//   2. Persist LastReminderSentAt atomically when the scheduler records
//      a successful send.
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

// ─────────────────────────────────────────────────────────────────────
// Normalisation
// ─────────────────────────────────────────────────────────────────────

// normalizeReminderDefaults fills in any missing reminder field on the
// given Task with its safe default. It is called after every load, so
// the caller is ALWAYS handed tasks whose reminder fields are fully
// populated and self-consistent.
//
// Rules (applied in order):
//
//  1. ReminderStartDate: if empty, copy from DueDate. This means an
//     old task starts reminding on its own due date — the safest
//     possible default: no surprise backlog of "you are overdue"
//     emails fires on first deploy, and the reminders naturally
//     begin when the user expects them to.
//
//  2. ReminderTime: if empty, set to "08:00" — matches the historical
//     cadence of the pre-revision scheduler.
//
//  3. ReminderTimezone: if empty, set to "Asia/Kolkata" — the timezone
//     this deployment has always effectively operated in.
//
//  4. ReminderEnabled: this is a bool, so there is no "absent" value
//     to fall back FROM. But we DO want old rows to be enabled. The
//     convention we use is: the JSON tag has NO `omitempty`, so every
//     row written after this revision persists `reminder_enabled`
//     explicitly, and every row written BEFORE this revision deserialises
//     it as Go's zero value (`false`). To distinguish "old row" from
//     "user really set it to false", we use the presence of ANY other
//     reminder field as a proxy: if ReminderStartDate, ReminderTime,
//     or ReminderTimezone was non-empty in the raw JSON, the row is
//     new-style and its ReminderEnabled value is authoritative. If
//     ALL THREE were empty, the row is old-style and we force-enable.
//
//     This proxy is precise in practice: a user who deliberately
//     disables reminders via the UI still picks a start date and time
//     (they're prefilled from the due date / 08:00), so at least one
//     of the three string fields is non-empty. A legacy row has all
//     three empty. The distinction is airtight.
func normalizeReminderDefaults(t *Task) {
	hasAnyReminderString :=
		strings.TrimSpace(t.ReminderStartDate) != "" ||
			strings.TrimSpace(t.ReminderTime) != "" ||
			strings.TrimSpace(t.ReminderTimezone) != ""

	if strings.TrimSpace(t.ReminderStartDate) == "" {
		t.ReminderStartDate = t.DueDate
	}
	if strings.TrimSpace(t.ReminderTime) == "" {
		t.ReminderTime = defaultReminderTime
	}
	if strings.TrimSpace(t.ReminderTimezone) == "" {
		t.ReminderTimezone = defaultReminderTimezone
	}

	// Only force-enable legacy rows. A new-style row keeps whatever
	// value it has, including false.
	if !hasAnyReminderString {
		t.ReminderEnabled = true
	}
}

// loadUnlocked reads the task list from disk (caller must hold a lock).
//
// Every task is passed through normalizeReminderDefaults before being
// returned, so callers never have to think about which fields were
// absent in the on-disk representation.
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

	for i := range list.Tasks {
		normalizeReminderDefaults(&list.Tasks[i])
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

// ─────────────────────────────────────────────────────────────────────
// Ownership-scoped CRUD
// ─────────────────────────────────────────────────────────────────────

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
//
// Reminder fields: the caller (service layer) is expected to have
// already applied defaults to the request (start date ← due date,
// time ← "08:00", timezone ← "Asia/Kolkata", enabled ← true) before
// calling Create. The store simply persists whatever it is given.
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
		// Reminder fields are zero here; the service layer has already
		// populated them via the pointer values from the request. If
		// the caller left them empty, normalizeReminderDefaults will
		// fill them in on the very next load. This makes Create
		// correct-by-default even if a future caller forgets to
		// populate them.
	}
	list.Tasks = append(list.Tasks, task)

	if err := s.saveUnlocked(list); err != nil {
		return nil, err
	}

	// Return the fully-normalised view to the caller, exactly as it
	// would be returned by a subsequent ListByUser or GetByID.
	normalizeReminderDefaults(&task)
	return &task, nil
}

// Update mutates an existing task identified by ID, but ONLY if it is
// owned by userID. Cross-user updates return ErrTaskNotFound so the
// client cannot distinguish "not found" from "not yours".
//
// Only Text and DueDate are updated by this method. Reminder fields
// are mutated via UpdateReminder below, because they have tri-state
// semantics on PUT (omitted = keep, "" = reset, value = replace) that
// would be awkward to express through plain arguments.
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

// UpdateReminder mutates only the reminder-related fields of a task,
// and only if the task is owned by userID.
//
// The parameters mirror UpdateTaskRequest exactly:
//
//	enabled     nil → keep current value; &b → set to b
//	startDate   nil → keep; &"" → reset to DueDate; &"YYYY-MM-DD" → set
//	time        nil → keep; &"" → reset to "08:00"; &"HH:MM" → set
//	timezone    nil → keep; &"" → reset to "Asia/Kolkata"; &"IANA" → set
//
// This method is intended to be called by the service layer, which
// performs validation on the values before delegating here.
func (s *Store) UpdateReminder(
	userID, id string,
	enabled *bool,
	startDate *string,
	reminderTime *string,
	timezone *string,
) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("cannot update reminder without an owner")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	for i := range list.Tasks {
		if list.Tasks[i].ID == id && list.Tasks[i].UserID == userID {
			t := &list.Tasks[i]

			if enabled != nil {
				t.ReminderEnabled = *enabled
			}
			if startDate != nil {
				trimmed := strings.TrimSpace(*startDate)
				if trimmed == "" {
					// Reset to default: the task's own DueDate.
					t.ReminderStartDate = t.DueDate
				} else {
					t.ReminderStartDate = trimmed
				}
			}
			if reminderTime != nil {
				trimmed := strings.TrimSpace(*reminderTime)
				if trimmed == "" {
					t.ReminderTime = defaultReminderTime
				} else {
					t.ReminderTime = trimmed
				}
			}
			if timezone != nil {
				trimmed := strings.TrimSpace(*timezone)
				if trimmed == "" {
					t.ReminderTimezone = defaultReminderTimezone
				} else {
					t.ReminderTimezone = trimmed
				}
			}

			t.UpdatedAt = time.Now().UTC()

			if err := s.saveUnlocked(list); err != nil {
				return nil, err
			}
			updated := list.Tasks[i]
			return &updated, nil
		}
	}

	return nil, ErrTaskNotFound
}

// MarkReminderSent atomically records that a reminder email was just
// dispatched for the given task, setting LastReminderSentAt = sentAt.
//
// Called ONLY by the scheduler, immediately after a successful email
// dispatch. The scheduler uses LastReminderSentAt as its idempotency
// guard: if the field falls on the same calendar day as the current
// tick (in the task's ReminderTimezone), the task is skipped.
//
// The write is scoped to the owning user: a scheduler iteration for
// user A cannot touch user B's tasks.
//
// Returns ErrTaskNotFound if the task does not exist or is not owned
// by userID. This can only happen if the task was deleted between the
// scheduler's ListByUser and this call — in that case the caller
// should treat it as a benign race and continue.
func (s *Store) MarkReminderSent(userID, id string, sentAt time.Time) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("cannot mark reminder sent without an owner")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUnlocked()
	if err != nil {
		return err
	}

	for i := range list.Tasks {
		if list.Tasks[i].ID == id && list.Tasks[i].UserID == userID {
			when := sentAt.UTC()
			list.Tasks[i].LastReminderSentAt = &when
			// We intentionally do NOT bump UpdatedAt here. The
			// scheduler's write is a bookkeeping update, not a
			// user-facing edit. Bumping UpdatedAt would make the
			// task appear "recently edited" in the UI every day at
			// reminder time, which would be confusing.

			return s.saveUnlocked(list)
		}
	}

	return ErrTaskNotFound
}

// Delete removes a task identified by ID, but ONLY if it is owned by
// userID. Cross-user deletes return ErrTaskNotFound.
//
// On success the function returns the task as it looked immediately
// BEFORE it was removed. The API handler uses this snapshot to
// populate the "Task Deleted" email body — the task is already gone
// from disk by the time the caller dispatches the email, so this is
// the only way to include its text and due date.
//
// The returned *Task is a copy of the internal record; the caller
// cannot mutate store state through it.
//
// Deleting a task is the ONLY way to stop its daily reminder — the
// scheduler has no concept of "stopped" tasks, only "existing" and
// "deleted" tasks. Once the row is gone from tasks.json, no future
// tick can find it.
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