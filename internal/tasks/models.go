package tasks

import "time"

// Task represents a single user task in data/tasks.json.
//
// UserID is the owner of the task. It is stamped from the authenticated
// JWT claim at creation time and NEVER accepted from the client body —
// clients send only Text and DueDate (plus the optional SendEmail flag).
type Task struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Text      string    `json:"text"`
	DueDate   string    `json:"due_date"` // YYYY-MM-DD
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
//	         (POST: email, matching the pre-existing behaviour)
//	&true    → client explicitly wants the confirmation email
//	         (this is what the "Email" button sends)
//	&false   → client explicitly wants a silent save
//	         (this is what the "Add" button sends)
//
// A plain bool would collapse nil and false into the same value,
// which would silently break clients that don't know the field exists.
type CreateTaskRequest struct {
	Text      string `json:"text"       example:"Call the dentist"`
	DueDate   string `json:"due_date"   example:"2026-11-05"`
	SendEmail *bool  `json:"send_email"`
}

// UpdateTaskRequest is the payload for PUT /tasks/{id}.
//
// SendEmail semantics match CreateTaskRequest, except the handler's
// default for nil is "silent" (matching the pre-existing behaviour —
// updates never used to email).
type UpdateTaskRequest struct {
	Text      string `json:"text"       example:"Call the dentist (rescheduled)"`
	DueDate   string `json:"due_date"   example:"2026-11-12"`
	SendEmail *bool  `json:"send_email"`
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
// CHANGED: added EmailSent so the frontend can pick the right toast
// wording when a delete-notification email is or isn't dispatched.
// The field is always emitted (no omitempty) so old clients that
// ignore it still work, and new clients can rely on it being present.
type MessageResponse struct {
	Success   bool   `json:"success"    example:"true"`
	Message   string `json:"message"    example:"Task deleted successfully"`
	EmailSent bool   `json:"email_sent" example:"true"`
}