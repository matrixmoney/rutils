package tasks

import (
	"errors"
	"fmt"
	"strings"
	"time"
	// time.LoadLocation validates the IANA timezone name that the user
	// supplies for a task's reminder. Any name that fails to load here
	// is rejected before it ever reaches the store.
)

// Sentinel errors returned by the service layer.
var (
	ErrEmptyText      = errors.New("task text is required")
	ErrTextTooLong    = errors.New("task text is too long (max 2000 characters)")
	ErrMissingDueDate = errors.New("due date is required (format: YYYY-MM-DD)")
	ErrInvalidDueDate = errors.New("due date must be in YYYY-MM-DD format")

	// ── Reminder-specific errors (new in this revision) ─────────────
	//
	// These are returned by validateReminderFields when the caller
	// supplies invalid values for the reminder fields on Create or
	// Update. They are surfaced to the client as HTTP 400 with a
	// human-readable message, exactly like the pre-existing
	// ErrEmptyText / ErrInvalidDueDate family.
	ErrInvalidReminderStartDate = errors.New("reminder_start_date must be in YYYY-MM-DD format")
	ErrReminderStartAfterDue    = errors.New("reminder_start_date cannot be after the due date")
	ErrInvalidReminderTime      = errors.New("reminder_time must be in HH:MM 24-hour format")
	ErrInvalidReminderTimezone  = errors.New("reminder_timezone must be a valid IANA timezone name")
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
//
// Reminder semantics (new in this revision):
//
// Every task carries its own reminder schedule. The service is
// responsible for turning the raw, possibly-partial request fields
// into a fully-populated set of reminder values, applying the following
// defaults before handing the task off to the store:
//
//	ReminderEnabled   → true when the client omits the flag
//	ReminderStartDate → the task's DueDate when the client omits it
//	ReminderTime      → "08:00" when the client omits it
//	ReminderTimezone  → "Asia/Kolkata" when the client omits it
//
// On Update, the same fields are tri-state (nil = keep, "" = reset to
// default, value = replace), and the service is what enforces that
// semantics by calling store.UpdateReminder with the appropriate
// pointers.
type Service struct {
	store *Store
}

// NewService wires the store into the service.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// ─────────────────────────────────────────────────────────────────────
// Reminder field validation helpers
// ─────────────────────────────────────────────────────────────────────

// validateReminderFields performs a full validation pass over the
// reminder fields that a client is about to attach to a task.
//
// The function accepts already-trimmed strings. Empty strings mean
// "use the default", and the caller is expected to have applied the
// defaults BEFORE calling this function — or, alternatively, to call
// this function with the raw request values and let it accept "" for
// "default applies". Both workflows are supported; the function
// simply skips validation for empty values.
//
// The cross-field rule is: if BOTH startDate and dueDate are non-empty,
// then startDate must not be after dueDate. This is enforced because a
// reminder that starts after the task is due would fire on a task the
// user is already past the due date for — the safest interpretation is
// that the user made a mistake, and we ask them to fix it.
//
// The reminder_timezone field is validated by attempting to load it via
// time.LoadLocation. This rejects typos ("Asia/Calcuta") and outdated
// names ("US/Pacific-New"), while accepting every IANA-canonical name.
func validateReminderFields(startDate, reminderTime, timezone, dueDate string) error {
	// ── reminder_start_date ─────────────────────────────────────
	if startDate != "" {
		parsedStart, err := time.Parse("2006-01-02", startDate)
		if err != nil {
			return ErrInvalidReminderStartDate
		}
		if dueDate != "" {
			parsedDue, err := time.Parse("2006-01-02", dueDate)
			if err == nil && parsedStart.After(parsedDue) {
				return ErrReminderStartAfterDue
			}
		}
	}

	// ── reminder_time ───────────────────────────────────────────
	if reminderTime != "" {
		// Go's time.Parse with the "15:04" layout accepts "20:00",
		// "09:30", "00:00", "23:59" — and rejects "24:00", "9:30"
		// (must be zero-padded), and "20:60". That is exactly the
		// strictness we want.
		if _, err := time.Parse("15:04", reminderTime); err != nil {
			return ErrInvalidReminderTime
		}
	}

	// ── reminder_timezone ───────────────────────────────────────
	if timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			return ErrInvalidReminderTimezone
		}
	}

	return nil
}

// applyReminderDefaults returns the effective reminder values for a
// task, applying the standard fallbacks for any empty field.
//
// It is used on the Create path, where "empty" unambiguously means
// "use the default" (there is no existing value to preserve).
//
// On the Update path, empty carries a different meaning (reset), and
// the store's UpdateReminder method handles the reset by re-applying
// the same defaults. So the values returned here are always
// fully-populated, non-empty strings — safe to persist directly.
func applyReminderDefaults(
	enabled bool,
	startDate, reminderTime, timezone, dueDate string,
) (bool, string, string, string) {
	if strings.TrimSpace(startDate) == "" {
		startDate = dueDate
	}
	if strings.TrimSpace(reminderTime) == "" {
		reminderTime = defaultReminderTime
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = defaultReminderTimezone
	}
	return enabled, startDate, reminderTime, timezone
}

// ─────────────────────────────────────────────────────────────────────
// CRUD
// ─────────────────────────────────────────────────────────────────────

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
//
// The reminder fields are optional. When the client omits a field, the
// service substitutes its default BEFORE calling the store, so the row
// that lands on disk is fully self-describing: there is never a stored
// task whose reminder_time is empty or whose reminder_start_date is
// absent. This property is what allows the scheduler's per-minute tick
// to read every task uniformly, with no "if empty then fall back to"
// branching at tick time.
//
// Defaults applied here (matching the CreateTaskRequest doc comment in
// models.go):
//
//	reminderEnabled   false or true as sent    → passed through
//	                  nil (client omitted)     → true
//	reminderStartDate "" (client omitted)      → the task's due_date
//	reminderTime      "" (client omitted)      → "08:00"
//	reminderTimezone  "" (client omitted)      → "Asia/Kolkata"
//
// The reminderStartDate value is validated against dueDate: it must
// be a valid YYYY-MM-DD and must not fall after dueDate.
func (s *Service) Create(
	userID, text, dueDate string,
	reminderEnabled *bool,
	reminderStartDate, reminderTime, reminderTimezone string,
) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}

	text, dueDate, err := validateFields(text, dueDate)
	if err != nil {
		return nil, err
	}

	// Normalise the raw inputs before validating them.
	reminderStartDate = strings.TrimSpace(reminderStartDate)
	reminderTime = strings.TrimSpace(reminderTime)
	reminderTimezone = strings.TrimSpace(reminderTimezone)

	if err := validateReminderFields(reminderStartDate, reminderTime, reminderTimezone, dueDate); err != nil {
		return nil, err
	}

	enabled := true
	if reminderEnabled != nil {
		enabled = *reminderEnabled
	}

	enabled, reminderStartDate, reminderTime, reminderTimezone = applyReminderDefaults(
		enabled, reminderStartDate, reminderTime, reminderTimezone, dueDate,
	)

	task, err := s.store.Create(userID, text, dueDate)
	if err != nil {
		return nil, err
	}

	// Populate the reminder fields on the freshly-created row. This is
	// a second write, but it is cheap (single file, tiny JSON) and it
	// keeps the Create signature in the store unchanged (which avoids
	// breaking any other caller).
	updated, err := s.store.UpdateReminder(
		userID, task.ID,
		&enabled,
		&reminderStartDate,
		&reminderTime,
		&reminderTimezone,
	)
	if err != nil {
		// The base row is already persisted; if the second write
		// fails, the row still has normalised defaults from
		// loadUnlocked. We log via the returned error but keep the
		// task (it exists and is usable).
		return task, fmt.Errorf("task created but reminder fields could not be persisted: %w", err)
	}

	return updated, nil
}

// Update validates and persists an edit to an existing task, but only
// if the task is owned by userID.
//
// Reminder field semantics on Update (all four are tri-state):
//
//	reminderEnabled    nil → keep current value
//	                   non-nil → replace
//
//	reminderStartDate  nil → keep current value
//	                   ptr to "" → reset to the task's due_date
//	                   ptr to "YYYY-MM-DD" → replace
//
//	reminderTime       nil → keep current value
//	                   ptr to "" → reset to "08:00"
//	                   ptr to "HH:MM" → replace
//
//	reminderTimezone   nil → keep current value
//	                   ptr to "" → reset to "Asia/Kolkata"
//	                   ptr to "IANA name" → replace
//
// Validation is applied to any non-nil value that is not the empty
// string; the empty string is treated as "reset", not as invalid.
//
// The function is a single atomic operation from the caller's point of
// view: text/due_date are updated first, then reminder fields are
// applied. If the reminder validation fails, the text/due_date update
// is rolled back by re-applying the previous snapshot — this keeps the
// two halves of the edit consistent.
func (s *Service) Update(
	userID, id, text, dueDate string,
	reminderEnabled *bool,
	reminderStartDate *string,
	reminderTime *string,
	reminderTimezone *string,
) (*Task, error) {
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

	// Resolve the values the reminder-validator needs to see:
	//   - The effective start date after the update (for the
	//     "start must be ≤ due" rule).
	//   - The effective time and timezone after the update (for
	//     syntax validation).
	//
	// We consult the current stored state for any field the client
	// has not overridden, so that the cross-field rule is always
	// checked against the post-update reality.
	current, err := s.store.GetByID(userID, id)
	if err != nil {
		return nil, err
	}

	effectiveStart := current.ReminderStartDate
	if reminderStartDate != nil {
		effectiveStart = strings.TrimSpace(*reminderStartDate)
	}
	effectiveTime := current.ReminderTime
	if reminderTime != nil {
		effectiveTime = strings.TrimSpace(*reminderTime)
	}
	effectiveTz := current.ReminderTimezone
	if reminderTimezone != nil {
		effectiveTz = strings.TrimSpace(*reminderTimezone)
	}

	// Validate the effective values. Empty strings (reset markers) are
	// skipped by validateReminderFields itself.
	if err := validateReminderFields(effectiveStart, effectiveTime, effectiveTz, dueDate); err != nil {
		return nil, err
	}

	// Apply text/due_date first.
	updated, err := s.store.Update(userID, id, text, dueDate)
	if err != nil {
		return nil, err
	}

	// Apply reminder fields. If ALL four pointers are nil, this is a
	// pure text/due_date edit and there is nothing to do.
	if reminderEnabled == nil && reminderStartDate == nil &&
		reminderTime == nil && reminderTimezone == nil {
		return updated, nil
	}

	updated, err = s.store.UpdateReminder(
		userID, id,
		reminderEnabled,
		reminderStartDate,
		reminderTime,
		reminderTimezone,
	)
	if err != nil {
		// Roll back the text/due_date edit to keep the two halves
		// of the update consistent. Best-effort — if the rollback
		// itself fails we return the original error.
		_, _ = s.store.Update(userID, id, current.Text, current.DueDate)
		return nil, fmt.Errorf("reminder update failed: %w", err)
	}

	return updated, nil
}

// Delete removes a task by ID, but only if it is owned by userID.
//
// On success the function returns the task as it looked immediately
// BEFORE it was removed. The handler uses this snapshot to populate
// the "Task Deleted" email — by the time the email is dispatched, the
// row is already gone from disk, so this is the only way to include
// its text and due date.
//
// Deleting a task is the ONLY way to stop its daily reminder. There is
// no "pause" or "end date" concept in the reminder model, by design:
// the user's stated requirement is "until the due date delete it".
func (s *Service) Delete(userID, id string) (*Task, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}
	if strings.TrimSpace(id) == "" {
		return nil, ErrTaskNotFound
	}
	return s.store.Delete(userID, id)
}

// ─────────────────────────────────────────────────────────────────────
// Scheduler support
// ─────────────────────────────────────────────────────────────────────

// MarkReminderSent is the scheduler's write path. It records the
// instant at which a reminder email was successfully dispatched for a
// given task, so that subsequent ticks on the same calendar day skip
// that task.
//
// This is a thin passthrough to the store, kept on the service so that
// the scheduler depends on a single, cohesive API (Service) rather than
// on the store directly. The scheduler iterates over all tasks of all
// users; it is natural for it to call svc.MarkReminderSent rather than
// reach through the service to the store.
func (s *Service) MarkReminderSent(userID, id string, sentAt time.Time) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("user ID is required")
	}
	if strings.TrimSpace(id) == "" {
		return ErrTaskNotFound
	}
	return s.store.MarkReminderSent(userID, id, sentAt)
}

// ─────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────

// validateFields trims whitespace and enforces the field rules.
// Returns the trimmed values on success.
//
// Note: this function is UNCHANGED from the pre-revision version. Its
// purpose is to validate only the core text/due_date fields. The
// reminder-specific validation lives in validateReminderFields above.
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