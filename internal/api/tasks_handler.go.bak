package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"file-converter/internal/auth"
	"file-converter/internal/tasks"
)

// TasksHandler handles HTTP requests for the /tasks endpoints.
type TasksHandler struct {
	service     *tasks.Service
	emailSender *auth.EmailSender
}

// NewTasksHandler creates a new TasksHandler.
func NewTasksHandler(service *tasks.Service, emailSender *auth.EmailSender) *TasksHandler {
	return &TasksHandler{service: service, emailSender: emailSender}
}

// writeTasksError emits a JSON error envelope consistent with the
// auth/image handlers' shape.
func writeTasksError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// List returns only the caller's tasks.
//
// @Summary      List all tasks
// @Description  Returns every task OWNED BY THE AUTHENTICATED USER, newest first. Requires Bearer JWT. Tasks belonging to other users are never returned.
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

	claims, ok := GetUserClaims(r.Context())
	if !ok || claims.UserID == "" {
		writeTasksError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	list, err := h.service.List(claims.UserID)
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

// Create inserts a new task owned by the caller and notifies them by email.
//
// @Summary      Create a task
// @Description  Creates a new task owned by the authenticated user. The owner is derived from the JWT — the client cannot set it. On success, a confirmation email is dispatched to the user's email address; EmailSent reflects whether the dispatch succeeded. The task is saved regardless of email outcome.
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

	claims, ok := GetUserClaims(r.Context())
	if !ok || claims.UserID == "" {
		writeTasksError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req tasks.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeTasksError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	task, err := h.service.Create(claims.UserID, req.Text, req.DueDate)
	if err != nil {
		writeTasksError(w, http.StatusBadRequest, mapTaskError(err))
		return
	}

	emailSent := h.dispatchTaskEmail(r.Context(), task)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(tasks.TaskResponse{
		Success:   true,
		Task:      *task,
		EmailSent: emailSent,
	})
}

// dispatchTaskEmail is unchanged from the previous implementation —
// it already reads claims.Email and claims.Name from context.
func (h *TasksHandler) dispatchTaskEmail(ctx context.Context, task *tasks.Task) bool {
	if h.emailSender == nil {
		return false
	}

	claims, ok := GetUserClaims(ctx)
	if !ok || claims.Email == "" {
		log.Printf("[tasks] skipping email for task %s: no user claims in context", task.ID)
		return false
	}

	done := make(chan error, 1)
	go func() {
		done <- h.emailSender.SendTaskNotification(
			claims.Email,
			claims.Name,
			task.Text,
			task.DueDate,
		)
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("[tasks] email dispatch failed for task %s to <%s>: %v",
				task.ID, claims.Email, err)
			return false
		}
		log.Printf("[tasks] email dispatched for task %s to <%s>",
			task.ID, claims.Email)
		return true

	case <-time.After(10 * time.Second):
		log.Printf("[tasks] email dispatch timed out (>10s) for task %s to <%s>; task is saved, email will not be sent",
			task.ID, claims.Email)
		return false

	case <-ctx.Done():
		log.Printf("[tasks] request cancelled while dispatching email for task %s to <%s>",
			task.ID, claims.Email)
		return false
	}
}

// Update mutates an existing task owned by the caller.
//
// @Summary      Update a task
// @Description  Updates the text and due date of a task OWNED BY THE AUTHENTICATED USER. Returns 404 for tasks owned by other users (indistinguishable from "not found" — no information leak). Requires Bearer JWT. No email is dispatched on updates.
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

	claims, ok := GetUserClaims(r.Context())
	if !ok || claims.UserID == "" {
		writeTasksError(w, http.StatusUnauthorized, "Unauthorized")
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

	task, err := h.service.Update(claims.UserID, id, req.Text, req.DueDate)
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
		Success:   true,
		Task:      *task,
		EmailSent: false,
	})
}

// Delete removes a task owned by the caller.
//
// @Summary      Delete a task
// @Description  Deletes a task OWNED BY THE AUTHENTICATED USER. Returns 404 for tasks owned by other users (indistinguishable from "not found" — no information leak). Requires Bearer JWT.
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

	claims, ok := GetUserClaims(r.Context())
	if !ok || claims.UserID == "" {
		writeTasksError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/tasks/")
	id = strings.Trim(id, "/")
	if id == "" {
		writeTasksError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	if err := h.service.Delete(claims.UserID, id); err != nil {
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