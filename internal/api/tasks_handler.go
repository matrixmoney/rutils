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

// resolveSendEmail collapses the tri-state *bool flag from a request
// body into a plain bool decision:
//
//	flag == nil       → return defaultWhenNil
//	flag != nil       → return *flag
//
// Kept as a tiny helper so the defaulting policy for each endpoint
// lives in exactly one place (the handler's call site), not scattered
// across the three functions.
//
// Defaulting table (documented at every call site):
//
//	Create: defaultWhenNil = true   (backwards-compat with pre-flag behaviour)
//	Update: defaultWhenNil = false  (backwards-compat with pre-flag behaviour)
//	Delete: defaultWhenNil = true   (new behaviour — delete is destructive)
func resolveSendEmail(flag *bool, defaultWhenNil bool) bool {
	if flag == nil {
		return defaultWhenNil
	}
	return *flag
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

// Create inserts a new task owned by the caller and (optionally) notifies
// them by email.
//
// The send_email body field is optional:
//
//	omitted / null  → email is sent (backwards-compat with the
//	                  pre-flag behaviour: create always emailed)
//	true            → email is sent   (this is the "Email" button)
//	false           → no email        (this is the "Add" button)
//
// The task is always saved. A failed email dispatch does NOT turn the
// response into an error — it only flips email_sent to false.
//
// @Summary      Create a task
// @Description  Creates a new task owned by the authenticated user. The owner is derived from the JWT — the client cannot set it. On success, a confirmation email is dispatched to the user's email address when `send_email` is true (default when omitted). EmailSent reflects whether the dispatch succeeded. The task is saved regardless of email outcome.
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

	// Default for POST: email. Matches the historical behaviour, so any
	// existing client that doesn't send the flag keeps getting its
	// confirmation email.
	emailSent := false
	if resolveSendEmail(req.SendEmail, true) {
		emailSent = h.dispatchTaskEmail(r.Context(), task)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(tasks.TaskResponse{
		Success:   true,
		Task:      *task,
		EmailSent: emailSent,
	})
}

// dispatchTaskEmail is unchanged from the previous implementation —
// it already reads claims.Email and claims.Name from context, and is
// now reused by the Update and Delete handlers as well.
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

// dispatchTaskUpdatedEmail sends the "your task was updated" email.
//
// Takes both the "before" and "after" snapshots so the email can show
// the actual diff. The before snapshot is loaded by the Update handler
// from the store BEFORE the edit is applied, so it cannot be forged by
// a malicious client.
func (h *TasksHandler) dispatchTaskUpdatedEmail(
	ctx context.Context,
	taskID string,
	oldText, oldDueDate,
	newText, newDueDate string,
) bool {
	if h.emailSender == nil {
		return false
	}

	claims, ok := GetUserClaims(ctx)
	if !ok || claims.Email == "" {
		log.Printf("[tasks] skipping update email for task %s: no user claims in context", taskID)
		return false
	}

	done := make(chan error, 1)
	go func() {
		done <- h.emailSender.SendTaskUpdatedNotification(
			claims.Email,
			claims.Name,
			oldText, oldDueDate,
			newText, newDueDate,
		)
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("[tasks] update email dispatch failed for task %s to <%s>: %v",
				taskID, claims.Email, err)
			return false
		}
		log.Printf("[tasks] update email dispatched for task %s to <%s>",
			taskID, claims.Email)
		return true

	case <-time.After(10 * time.Second):
		log.Printf("[tasks] update email dispatch timed out (>10s) for task %s to <%s>; task is saved, email will not be sent",
			taskID, claims.Email)
		return false

	case <-ctx.Done():
		log.Printf("[tasks] request cancelled while dispatching update email for task %s to <%s>",
			taskID, claims.Email)
		return false
	}
}

// dispatchTaskDeletedEmail sends the "your task was deleted" email.
//
// Takes the deleted task's snapshot (returned by Store.Delete) so the
// email can show exactly what was removed.
func (h *TasksHandler) dispatchTaskDeletedEmail(
	ctx context.Context,
	taskID string,
	taskText, dueDate string,
) bool {
	if h.emailSender == nil {
		return false
	}

	claims, ok := GetUserClaims(ctx)
	if !ok || claims.Email == "" {
		log.Printf("[tasks] skipping delete email for task %s: no user claims in context", taskID)
		return false
	}

	done := make(chan error, 1)
	go func() {
		done <- h.emailSender.SendTaskDeletedNotification(
			claims.Email,
			claims.Name,
			taskText,
			dueDate,
		)
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("[tasks] delete email dispatch failed for task %s to <%s>: %v",
				taskID, claims.Email, err)
			return false
		}
		log.Printf("[tasks] delete email dispatched for task %s to <%s>",
			taskID, claims.Email)
		return true

	case <-time.After(10 * time.Second):
		log.Printf("[tasks] delete email dispatch timed out (>10s) for task %s to <%s>; task is deleted, email will not be sent",
			taskID, claims.Email)
		return false

	case <-ctx.Done():
		log.Printf("[tasks] request cancelled while dispatching delete email for task %s to <%s>",
			taskID, claims.Email)
		return false
	}
}

// Update mutates an existing task owned by the caller and (optionally)
// notifies them by email.
//
// The send_email body field is optional:
//
//	omitted / null  → NO email (backwards-compat with the pre-flag
//	                  behaviour: updates never used to email)
//	true            → email is sent   (this is the "Email" button)
//	false           → no email        (this is the "Update" button)
//
// The email shows the OLD values (strikethrough) above the NEW values
// so the recipient can see exactly what changed. The "old" values are
// loaded server-side before the edit is applied — the client cannot
// forge them.
//
// @Summary      Update a task
// @Description  Updates the text and due date of a task OWNED BY THE AUTHENTICATED USER. Returns 404 for tasks owned by other users (indistinguishable from "not found" — no information leak). Requires Bearer JWT. When `send_email` is true, an "updated" email is dispatched showing the before/after diff. `email_sent` reflects whether the dispatch succeeded. The task is saved regardless of email outcome.
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

	// Snapshot the OLD state before mutating anything. This is what
	// makes the update-email honest: the "before" side is exactly what
	// was on disk a moment ago, not whatever the client claims.
	oldTask, err := h.service.GetByID(claims.UserID, id)
	if err != nil {
		if errors.Is(err, tasks.ErrTaskNotFound) {
			writeTasksError(w, http.StatusNotFound, "Task not found")
			return
		}
		writeTasksError(w, http.StatusInternalServerError, "Failed to load task: "+err.Error())
		return
	}

	updated, err := h.service.Update(claims.UserID, id, req.Text, req.DueDate)
	if err != nil {
		if errors.Is(err, tasks.ErrTaskNotFound) {
			writeTasksError(w, http.StatusNotFound, "Task not found")
			return
		}
		writeTasksError(w, http.StatusBadRequest, mapTaskError(err))
		return
	}

	// Default for PUT: no email. Matches the historical behaviour —
	// updates were silent before the flag existed.
	emailSent := false
	if resolveSendEmail(req.SendEmail, false) {
		emailSent = h.dispatchTaskUpdatedEmail(
			r.Context(),
			updated.ID,
			oldTask.Text, oldTask.DueDate,
			updated.Text, updated.DueDate,
		)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks.TaskResponse{
		Success:   true,
		Task:      *updated,
		EmailSent: emailSent,
	})
}

// Delete removes a task owned by the caller and (optionally) notifies
// them by email.
//
// The send_email field is read from the query string:
//
//	omitted / anything else  → email is sent   (NEW default — delete
//	                          is destructive, so we notify by default)
//	"true" / "1"             → email is sent   (explicit)
//	"false" / "0"            → no email        (explicit)
//
// The email shows the text and due date of the deleted task so the
// user has a record of what was removed.
//
// @Summary      Delete a task
// @Description  Deletes a task OWNED BY THE AUTHENTICATED USER. Returns 404 for tasks owned by other users (indistinguishable from "not found" — no information leak). Requires Bearer JWT. A "deleted" email is dispatched by default (pass ?send_email=false to suppress it). `email_sent` reflects whether the dispatch succeeded. The task is deleted regardless of email outcome.
// @Tags         Tasks
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "Task ID"
// @Param        send_email query string false "Set to 'false' to suppress the deletion notification email (default: true)"
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

	// Snapshot first so we can distinguish "not found" from "delete
	// failed". Also keeps the flow symmetrical with Update and gives
	// the caller a clean 404 before we touch the store.
	if _, err := h.service.GetByID(claims.UserID, id); err != nil {
		if errors.Is(err, tasks.ErrTaskNotFound) {
			writeTasksError(w, http.StatusNotFound, "Task not found")
			return
		}
		writeTasksError(w, http.StatusInternalServerError, "Failed to load task: "+err.Error())
		return
	}

	// Delete returns the exact task that was removed — use THIS for
	// the email, not the earlier GetByID snapshot. If a concurrent
	// request had modified the task in the millisecond between the
	// two calls, only the deleted snapshot reflects what actually
	// got deleted.
	deleted, err := h.service.Delete(claims.UserID, id)
	if err != nil {
		if errors.Is(err, tasks.ErrTaskNotFound) {
			writeTasksError(w, http.StatusNotFound, "Task not found")
			return
		}
		writeTasksError(w, http.StatusInternalServerError, "Failed to delete task: "+err.Error())
		return
	}

	// Default for DELETE: email. Delete is destructive — if anything
	// unexpected happens, we want the user to have a record of what
	// disappeared.
	sendEmail := resolveDeleteSendEmail(r.URL.Query().Get("send_email"))
	emailSent := false
	if sendEmail {
		emailSent = h.dispatchTaskDeletedEmail(
			r.Context(),
			deleted.ID,
			deleted.Text,
			deleted.DueDate,
		)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks.MessageResponse{
		Success:   true,
		Message:   "Task deleted successfully",
		EmailSent: emailSent,
	})
}

// resolveDeleteSendEmail parses the ?send_email= query param for the
// DELETE endpoint.
//
// Default when the param is absent (or unrecognised): true — delete
// is destructive and we notify by default.
//
// Accepts the same truthy/falsy spellings as most HTTP query-param
// parsers, so that clients can use whatever feels natural.
func resolveDeleteSendEmail(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
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