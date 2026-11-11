package tasks

import "time"

// Task represents a single user task in data/tasks.json.
//
// UserID is the owner of the task. It is stamped from the authenticated
// JWT claim at creation time and NEVER accepted from the client body —
// clients send only Text and DueDate (plus the optional SendEmail flag
// and the reminder fields documented below).
//
// ────────────────────────────────────────────────────────────────────
// REMINDER FIELDS (new in this revision)
// ────────────────────────────────────────────────────────────────────
//
// Every task carries its own per-task reminder schedule. The semantics
// are:
//
//   - ReminderEnabled is the master on/off switch. When false, the
//     scheduler will never email this task, regardless of the other
//     reminder fields.
//
//   - ReminderStartDate is the date (YYYY-MM-DD) from which reminders
//     begin. The first email fires on this date at ReminderTime. Every
//     subsequent day thereafter, another email fires at ReminderTime,
//     forever — until the task is DELETED. There is NO automatic stop
//     at the due date, and no "N-day window". Deleting the task is the
//     only way to stop the reminders.
//
//   - ReminderTime is the time-of-day (HH:MM, 24-hour) at which the
//     reminder email fires on every active day.
//
//   - ReminderTimezone is the IANA timezone name (e.g. "Asia/Kolkata",
//     "America/New_York") in which ReminderStartDate + ReminderTime are
//     interpreted. This makes "20:00" unambiguous even if the server
//     runs in a different zone or the user later travels.
//
//   - LastReminderSentAt is written ONLY by the scheduler. It records
//     the instant the most recent reminder email was successfully
//     dispatched for this task. The scheduler uses it as an idempotency
//     guard: if LastReminderSentAt falls on the same calendar day as
//     `now` (in ReminderTimezone), the task is skipped for that tick.
//     This is what prevents a per-minute ticker from firing 1,440
//     emails per day.
//
// Defaults when a field is absent (e.g. old rows written before this
// revision):
//
//   - ReminderEnabled     → true
//   - ReminderStartDate   → the task's DueDate
//   - ReminderTime        → "08:00"
//   - ReminderTimezone    → "Asia/Kolkata"
//   - LastReminderSentAt  → nil
//
// These defaults are applied by Store.normalizeReminderDefaults() at
// load time and are NEVER persisted back automatically — only an
// explicit Create/Update writes them to disk.
// ────────────────────────────────────────────────────────────────────
type Task struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Text      string    `json:"text"`
	DueDate   string    `json:"due_date"` // YYYY-MM-DD
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// ── Reminder fields (new) ────────────────────────────────────
	ReminderEnabled    bool       `json:"reminder_enabled"`
	ReminderStartDate  string     `json:"reminder_start_date,omitempty"` // YYYY-MM-DD
	ReminderTime       string     `json:"reminder_time,omitempty"`       // "HH:MM" 24-hour
	ReminderTimezone   string     `json:"reminder_timezone,omitempty"`   // IANA tz name
	LastReminderSentAt *time.Time `json:"last_reminder_sent_at,omitempty"`
}

// TaskList is the on-disk structure of data/tasks.json.
type TaskList struct {
	Tasks []Task `json:"tasks"`
}

// CreateTaskRequest is the payload for POST /tasks.
//
// SendEmail is a pointer so the handler can distinguish three cases:
//
//	nil      → client did not specify; handler uses its default
//	           (POST: email, matching the pre-existing behaviour)
//	&true    → client explicitly wants the confirmation email
//	           (this is what the "Email" button sends)
//	&false   → client explicitly wants a silent save
//	           (this is what the "Add" button sends)
//
// A plain bool would collapse nil and false into the same value,
// which would silently break clients that don't know the field exists.
//
// ────────────────────────────────────────────────────────────────────
// REMINDER FIELDS — all optional, all have safe defaults
// ────────────────────────────────────────────────────────────────────
//
//	ReminderEnabled    → *bool so nil means "true" (scheduler WILL email)
//	ReminderStartDate  → empty string means "use DueDate"
//	ReminderTime       → empty string means "08:00"
//	ReminderTimezone   → empty string means "Asia/Kolkata"
//
// Clients that don't know about these fields keep working unchanged:
// every field is omitted → defaults apply → the task gets a daily
// reminder starting on its due date at 08:00 IST.
type CreateTaskRequest struct {
	Text      string `json:"text"       example:"Call the dentist"`
	DueDate   string `json:"due_date"   example:"2026-11-05"`
	SendEmail *bool  `json:"send_email"`

	// ── Reminder fields (new, all optional) ─────────────────────
	ReminderEnabled   *bool  `json:"reminder_enabled,omitempty"`
	ReminderStartDate string `json:"reminder_start_date,omitempty"`
	ReminderTime      string `json:"reminder_time,omitempty"`
	ReminderTimezone  string `json:"reminder_timezone,omitempty"`
}

// UpdateTaskRequest is the payload for PUT /tasks/{id}.
//
// SendEmail semantics match CreateTaskRequest, except the handler's
// default for nil is "silent" (matching the pre-existing behaviour —
// updates never used to email).
//
// Reminder field semantics: every reminder field is optional. The
// tri-state for each is:
//
//	nil pointer       → leave the stored value UNCHANGED
//	non-nil pointer   → replace the stored value with the new one
//	                    (even if the pointed-to value is zero)
//
// For the string fields (ReminderStartDate / ReminderTime /
// ReminderTimezone), the tri-state is expressed via the **pointer**
// rather than an empty string, because we need to distinguish:
//
//	omitted entirely          → keep existing stored value
//	sent as ""                → reset to default
//	                          (empty → DueDate / "08:00" / "Asia/Kolkata")
//	sent as "2026-12-01" etc. → replace with the new value
//
// Using a pointer for every reminder field makes this tri-state
// explicit at the Go type level instead of relying on the JSON
// decoder's behaviour with empty strings.
type UpdateTaskRequest struct {
	Text      string `json:"text"       example:"Call the dentist (rescheduled)"`
	DueDate   string `json:"due_date"   example:"2026-11-12"`
	SendEmail *bool  `json:"send_email"`

	// ── Reminder fields (new, all optional) ─────────────────────
	ReminderEnabled   *bool   `json:"reminder_enabled,omitempty"`
	ReminderStartDate *string `json:"reminder_start_date,omitempty"`
	ReminderTime      *string `json:"reminder_time,omitempty"`
	ReminderTimezone  *string `json:"reminder_timezone,omitempty"`
}

// TaskResponse is the payload returned by create / update endpoints.
//
// EmailSent is always emitted (no omitempty) so the frontend can
// reliably distinguish "saved + emailed" from "saved silently" and
// "saved but email failed".
type TaskResponse struct {
	Success   bool `json:"success"    example:"true"`
	Task      Task `json:"task"`
	EmailSent bool `json:"email_sent" example:"true"`
}

// TasksListResponse is the payload returned by GET /tasks.
type TasksListResponse struct {
	Success bool   `json:"success" example:"true"`
	Tasks   []Task `json:"tasks"`
}

// MessageResponse is the payload returned by DELETE /tasks/{id}.
//
// EmailSent reflects whether the deletion-notification email was
// dispatched. The field is always emitted (no omitempty) so old
// clients that ignore it still work, and new clients can rely on it
// being present.
type MessageResponse struct {
	Success   bool   `json:"success"    example:"true"`
	Message   string `json:"message"    example:"Task deleted successfully"`
	EmailSent bool   `json:"email_sent" example:"true"`
}
