package tasks

import "time"

// Task represents a single user task in data/tasks.json.
//
// UserID is the owner of the task. It is stamped from the authenticated
// JWT claim at creation time and NEVER accepted from the client body —
// clients send only Text and DueDate.
type Task struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Text      string    `json:"text"`
	DueDate   string    `json:"due_date"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TaskList struct {
	Tasks []Task `json:"tasks"`
}

type CreateTaskRequest struct {
	Text    string `json:"text" example:"Call the dentist"`
	DueDate string `json:"due_date" example:"2026-11-05"`
}

type UpdateTaskRequest struct {
	Text    string `json:"text" example:"Call the dentist (rescheduled)"`
	DueDate string `json:"due_date" example:"2026-11-12"`
}

type TaskResponse struct {
	Success   bool `json:"success" example:"true"`
	Task      Task `json:"task"`
	EmailSent bool `json:"email_sent" example:"true"`
}

type TasksListResponse struct {
	Success bool   `json:"success" example:"true"`
	Tasks   []Task `json:"tasks"`
}

type MessageResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Task deleted successfully"`
}