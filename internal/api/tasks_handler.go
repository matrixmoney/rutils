package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"file-converter/internal/tasks"
)

// TasksHandler handles HTTP requests for the /tasks endpoints.
type TasksHandler struct {
	service *tasks.Service
}

// NewTasksHandler creates a new TasksHandler.
func NewTasksHandler(service *tasks.Service) *TasksHandler {
	return &TasksHandler{service: service}
}

// writeTasksError emits a JSON error envelope consistent with the
// auth/image handlers' shape.
func writeTasksError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// List returns every saved task.
//
// @Summary      List all tasks
// @Description  Returns every saved task, newest first. Requires Bearer JWT.
// @Tags         Tasks
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} tasks.TasksListResponse "List of tasks"
// @Failure      401 {object} map[string]string "Unauthorized"
// @Failure      500 {object} map[string]string "Internal server error"
// @Router       /tasks [get]
func (h *TasksHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeTasksError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	list, err := h.service.List()
	if err != nil {
		writeTasksError(w, http.StatusInternalServerError, "Failed to load tasks: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks.TasksListResponse{
		Success: true,
		Tasks:   list,
	})
}

// Create inserts a new task.
//
// @Summary      Create a task
// @Description  Creates a new task with text and a due date (YYYY-MM-DD). Requires Bearer JWT.
// @Tags         Tasks
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body tasks.CreateTaskRequest true "Create Task Request"
// @Success      201 {object} tasks.TaskResponse "Task created"
// @Failure      400 {object} map[string]string "Validation error"
// @Failure      401 {object} map[string]string "Unauthorized"
// @Failure      500 {object} map[string]string "Internal server error"
// @Router       /tasks [post]
func (h *TasksHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeTasksError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req tasks.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeTasksError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	task, err := h.service.Create(req.Text, req.DueDate)
	if err != nil {
		writeTasksError(w, http.StatusBadRequest, mapTaskError(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(tasks.TaskResponse{
		Success: true,
		Task:    *task,
	})
}

// Update mutates an existing task.
//
// @Summary      Update a task
// @Description  Updates the text and due date of a task. Requires Bearer JWT.
// @Tags         Tasks
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id      path string                  true "Task ID (e.g. tsk_ab12cd34)"
// @Param        request body tasks.UpdateTaskRequest true "Update Task Request"
// @Success      200 {object} tasks.TaskResponse "Task updated"
// @Failure      400 {object} map[string]string "Validation error"
// @Failure      401 {object} map[string]string "Unauthorized"
// @Failure      404 {object} map[string]string "Task not found"
// @Failure      500 {object} map[string]string "Internal server error"
// @Router       /tasks/{id} [put]
func (h *TasksHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeTasksError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/tasks/")
	id = strings.Trim(id, "/")
	if id == "" {
		writeTasksError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	var req tasks.UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeTasksError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	task, err := h.service.Update(id, req.Text, req.DueDate)
	if err != nil {
		if errors.Is(err, tasks.ErrTaskNotFound) {
			writeTasksError(w, http.StatusNotFound, "Task not found")
			return
		}
		writeTasksError(w, http.StatusBadRequest, mapTaskError(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks.TaskResponse{
		Success: true,
		Task:    *task,
	})
}

// Delete removes a task.
//
// @Summary      Delete a task
// @Description  Deletes the task identified by ID. Requires Bearer JWT.
// @Tags         Tasks
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "Task ID"
// @Success      200 {object} tasks.MessageResponse "Task deleted"
// @Failure      401 {object} map[string]string "Unauthorized"
// @Failure      404 {object} map[string]string "Task not found"
// @Failure      500 {object} map[string]string "Internal server error"
// @Router       /tasks/{id} [delete]
func (h *TasksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeTasksError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/tasks/")
	id = strings.Trim(id, "/")
	if id == "" {
		writeTasksError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	if err := h.service.Delete(id); err != nil {
		if errors.Is(err, tasks.ErrTaskNotFound) {
			writeTasksError(w, http.StatusNotFound, "Task not found")
			return
		}
		writeTasksError(w, http.StatusInternalServerError, "Failed to delete task: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks.MessageResponse{
		Success: true,
		Message: "Task deleted successfully",
	})
}

// mapTaskError translates service errors into human-readable messages.
// The service always returns one of the sentinel errors defined in
// internal/tasks/service.go, so this is a straightforward mapping.
func mapTaskError(err error) string {
	switch {
	case errors.Is(err, tasks.ErrEmptyText):
		return "Task text cannot be empty"
	case errors.Is(err, tasks.ErrTextTooLong):
		return "Task text is too long (max 2000 characters)"
	case errors.Is(err, tasks.ErrMissingDueDate):
		return "Due date is required (format: YYYY-MM-DD)"
	case errors.Is(err, tasks.ErrInvalidDueDate):
		return "Due date must be a valid YYYY-MM-DD date"
	case errors.Is(err, tasks.ErrTaskNotFound):
		return "Task not found"
	default:
		return err.Error()
	}
}