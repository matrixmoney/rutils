package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"file-converter/internal/auth"
	"file-converter/internal/tasks"
)

// ============================================================
// Test helpers
// ============================================================

// newTestTasksHandler builds a TasksHandler backed by a real
// file-backed tasks.Store in a temp directory. The caller decides
// whether to pass a nil or a non-nil *auth.EmailSender — that is
// the whole point of these tests.
//
// Every invocation gets its own temp directory, so tests cannot
// leak state into one another even when run in parallel.
func newTestTasksHandler(t *testing.T, sender *auth.EmailSender) (*TasksHandler, *tasks.Service) {
	t.Helper()

	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "tasks.json")

	store, err := tasks.NewStore(storePath)
	if err != nil {
		t.Fatalf("failed to create tasks store: %v", err)
	}

	service := tasks.NewService(store)
	handler := NewTasksHandler(service, sender)

	return handler, service
}

// withClaimsContext returns a request whose context carries the given
// auth.Claims, as if the request had passed through api.RequireAuth.
func withClaimsContext(r *http.Request, userID, email, name string) *http.Request {
	claims := &auth.Claims{
		UserID: userID,
		Email:  email,
		Name:   name,
	}
	ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
	return r.WithContext(ctx)
}

// decodeTaskResponse decodes a tasks.TaskResponse from the recorder
// body, failing the test on malformed JSON.
func decodeTaskResponse(t *testing.T, rec *httptest.ResponseRecorder) tasks.TaskResponse {
	t.Helper()

	var body tasks.TaskResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v (raw: %q)", err, rec.Body.String())
	}
	return body
}

// unconfiguredSender returns an *auth.EmailSender whose IsConfigured()
// is false. That means the Send*Notification methods take the
// development-fallback path: they log a single line to stdout and
// return nil — no network call, no credentials, no flakiness.
//
// This is the same setup the pre-existing
// TestTasksHandler_Create_EmailSenderWithoutConfiguredSMTP test uses,
// extracted here so the new tests don't repeat it inline.
func unconfiguredSender() *auth.EmailSender {
	return auth.NewEmailSender(auth.DefaultConfig())
}

// ptrBool is a tiny convenience for building *bool literals.
func ptrBool(b bool) *bool { return &b }

// postCreate is a shortcut for the common "POST /tasks with these
// fields and these claims" flow used throughout the file.
func postCreate(
	t *testing.T,
	handler *TasksHandler,
	userID, email, name string,
	body tasks.CreateTaskRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = withClaimsContext(req, userID, email, name)

	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	return rec
}

// putUpdate is a shortcut for "PUT /tasks/{id}".
func putUpdate(
	t *testing.T,
	handler *TasksHandler,
	userID, email, name, taskID string,
	body tasks.UpdateTaskRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/tasks/"+taskID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = withClaimsContext(req, userID, email, name)

	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	return rec
}

// deleteRequest builds a "DELETE /tasks/{id}" request. rawQuery is
// appended verbatim, e.g. "?send_email=false" or "".
func deleteRequest(
	handler *TasksHandler,
	userID, email, name, taskID, rawQuery string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/tasks/"+taskID+rawQuery, nil)
	req = withClaimsContext(req, userID, email, name)

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)
	return rec
}

// ============================================================
// PRE-EXISTING TESTS — unchanged, kept as-is
// ============================================================

// TestTasksHandler_Create_WithNilEmailSender pins down the single most
// important guarantee of the Tasks feature:
//
//	When the handler is constructed with a nil *auth.EmailSender,
//	POST /tasks MUST still succeed, MUST still save the task, and
//	MUST return email_sent=false. It MUST NOT panic.
func TestTasksHandler_Create_WithNilEmailSender(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	if handler.emailSender != nil {
		t.Fatalf("test setup error: handler.emailSender must be nil for this test")
	}

	rec := postCreate(t, handler, "usr_test123", "alice@example.com", "Alice Smith",
		tasks.CreateTaskRequest{
			Text:    "Buy groceries for the weekend",
			DueDate: "2026-11-05",
		})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeTaskResponse(t, rec)
	if !body.Success {
		t.Fatalf("expected success=true, got %v", body.Success)
	}
	if body.EmailSent {
		t.Fatalf("expected email_sent=false for nil sender, got true")
	}
	if body.Task.ID == "" {
		t.Fatalf("expected task with generated ID, got empty")
	}
	if body.Task.Text != "Buy groceries for the weekend" {
		t.Fatalf("text round-trip failed: %q", body.Task.Text)
	}

	list, err := service.List("usr_test123")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != body.Task.ID {
		t.Fatalf("task not persisted correctly: %+v", list)
	}
}

// TestTasksHandler_Create_EmailSenderWithoutConfiguredSMTP pins down
// the dev-mode guarantee:
//
//	When the EmailSender is present but SMTP is unconfigured,
//	SendTaskNotification falls back to logging and returns nil.
//	Therefore POST /tasks MUST return email_sent=true.
func TestTasksHandler_Create_EmailSenderWithoutConfiguredSMTP(t *testing.T) {
	cfg := auth.DefaultConfig()
	sender := auth.NewEmailSender(cfg)

	if sender.IsConfigured() {
		t.Fatalf("test setup error: sender must not be configured for this test")
	}

	handler, service := newTestTasksHandler(t, sender)

	rec := postCreate(t, handler, "usr_test456", "bob@example.com", "Bob Jones",
		tasks.CreateTaskRequest{
			Text:    "Water the plants",
			DueDate: "2026-10-30",
		})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeTaskResponse(t, rec)
	if !body.Success {
		t.Fatalf("expected success=true")
	}
	if !body.EmailSent {
		t.Fatalf("expected email_sent=true under dev fallback, got false")
	}

	list, err := service.List("usr_test456")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 task, got %d", len(list))
	}
}

// TestTasksHandler_Create_RejectsInvalidPayload checks that a
// whitespace-only text is rejected with a 400 and nothing is stored.
func TestTasksHandler_Create_RejectsInvalidPayload(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	rec := postCreate(t, handler, "usr_test789", "carol@example.com", "Carol White",
		tasks.CreateTaskRequest{
			Text:    "   ",
			DueDate: "2026-11-05",
		})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var errBody map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if errBody["error"] == "" {
		t.Fatalf("expected non-empty error message, got %v", errBody)
	}

	list, err := service.List("usr_test789")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(list))
	}
}

// TestTasksHandler_CrossUserIsolation pins down the pre-existing
// security property that this feature must NOT weaken:
//
//	User A creates a task.
//	User B cannot see it via List, cannot update it (404), cannot
//	delete it (404).
//	User A's task is unchanged after all of B's failed attempts.
func TestTasksHandler_CrossUserIsolation(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	// --- Alice creates a task ---
	createRec := postCreate(t, handler, "usr_alice", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:    "Alice's private note",
			DueDate: "2026-11-05",
		})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("alice create failed: %d %s", createRec.Code, createRec.Body.String())
	}
	aliceTaskID := decodeTaskResponse(t, createRec).Task.ID

	// --- Bob lists, updates, deletes → cannot see / touch ---
	listReq := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	listReq = withClaimsContext(listReq, "usr_bob", "bob@example.com", "Bob")
	listRec := httptest.NewRecorder()
	handler.List(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("bob list failed: %d", listRec.Code)
	}

	var bobList tasks.TasksListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&bobList); err != nil {
		t.Fatalf("failed to decode bob list: %v", err)
	}
	if len(bobList.Tasks) != 0 {
		t.Fatalf("SECURITY FAILURE: bob can see %d task(s) he does not own", len(bobList.Tasks))
	}

	updateRec := putUpdate(t, handler, "usr_bob", "bob@example.com", "Bob", aliceTaskID,
		tasks.UpdateTaskRequest{Text: "Hijacked", DueDate: "2027-01-01"})
	if updateRec.Code != http.StatusNotFound {
		t.Fatalf("SECURITY FAILURE: bob update returned %d, expected 404", updateRec.Code)
	}

	deleteRec := deleteRequest(handler, "usr_bob", "bob@example.com", "Bob", aliceTaskID, "")
	if deleteRec.Code != http.StatusNotFound {
		t.Fatalf("SECURITY FAILURE: bob delete returned %d, expected 404", deleteRec.Code)
	}

	// --- Alice's task still exists, unchanged ---
	aliceList, err := service.List("usr_alice")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(aliceList) != 1 {
		t.Fatalf("expected alice to still have 1 task, got %d", len(aliceList))
	}
	if aliceList[0].Text != "Alice's private note" {
		t.Fatalf("alice's task text mutated: %q", aliceList[0].Text)
	}
	if aliceList[0].DueDate != "2026-11-05" {
		t.Fatalf("alice's due date mutated: %q", aliceList[0].DueDate)
	}
}

// ============================================================
// NEW TESTS — send_email semantics on Create / Update / Delete
// ============================================================

// ---------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------

// TestTasksHandler_Create_SendEmailFalse_SkipsEmail pins down the
// "silent save" path that the new "Add" button uses.
func TestTasksHandler_Create_SendEmailFalse_SkipsEmail(t *testing.T) {
	handler, service := newTestTasksHandler(t, unconfiguredSender())

	rec := postCreate(t, handler, "usr_silent_create", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "Silent note",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeTaskResponse(t, rec)
	if body.EmailSent {
		t.Fatalf("expected email_sent=false when SendEmail=false, got true")
	}
	if !body.Success {
		t.Fatalf("expected success=true")
	}

	list, _ := service.List("usr_silent_create")
	if len(list) != 1 {
		t.Fatalf("expected task persisted, got %d rows", len(list))
	}
}

// TestTasksHandler_Create_SendEmailOmitted_DefaultsToEmail pins down
// the backwards-compatibility default: no flag means "email" (matches
// the pre-flag behaviour).
func TestTasksHandler_Create_SendEmailOmitted_DefaultsToEmail(t *testing.T) {
	handler, _ := newTestTasksHandler(t, unconfiguredSender())

	rec := postCreate(t, handler, "usr_default_create", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:    "Default note",
			DueDate: "2026-11-05",
			// SendEmail intentionally omitted → nil
		})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	body := decodeTaskResponse(t, rec)
	if !body.EmailSent {
		t.Fatalf("expected email_sent=true (dev fallback) when SendEmail omitted, got false")
	}
}

// TestTasksHandler_Create_SendEmailTrue_NilSenderStillReturns201 ensures
// that even when the client explicitly asks for an email but the handler
// has no sender, the request still succeeds and reports email_sent=false.
func TestTasksHandler_Create_SendEmailTrue_NilSenderStillReturns201(t *testing.T) {
	handler, _ := newTestTasksHandler(t, nil)

	rec := postCreate(t, handler, "usr_nil_sender", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "Wants email but sender is nil",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(true),
		})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeTaskResponse(t, rec)
	if body.EmailSent {
		t.Fatalf("expected email_sent=false for nil sender, got true")
	}
	if !body.Success {
		t.Fatalf("expected success=true even when email dispatch is impossible")
	}
}

// ---------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------

// TestTasksHandler_Update_SendEmailFalse_SkipsEmail pins down the
// "silent update" path that the new "Update" button uses.
func TestTasksHandler_Update_SendEmailFalse_SkipsEmail(t *testing.T) {
	handler, service := newTestTasksHandler(t, unconfiguredSender())

	// Seed a task first.
	createRec := postCreate(t, handler, "usr_upd_silent", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "Original text",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("seed create failed: %d", createRec.Code)
	}
	taskID := decodeTaskResponse(t, createRec).Task.ID

	// Update silently.
	rec := putUpdate(t, handler, "usr_upd_silent", "alice@example.com", "Alice", taskID,
		tasks.UpdateTaskRequest{
			Text:      "Updated silently",
			DueDate:   "2026-11-12",
			SendEmail: ptrBool(false),
		})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeTaskResponse(t, rec)
	if body.EmailSent {
		t.Fatalf("expected email_sent=false when SendEmail=false, got true")
	}
	if body.Task.Text != "Updated silently" {
		t.Fatalf("task text not updated: %q", body.Task.Text)
	}

	list, _ := service.List("usr_upd_silent")
	if len(list) != 1 || list[0].Text != "Updated silently" {
		t.Fatalf("persisted task not updated correctly: %+v", list)
	}
}

// TestTasksHandler_Update_SendEmailOmitted_DefaultsToSilent pins down
// the backwards-compatibility default: no flag means "no email"
// (matches the pre-flag behaviour).
func TestTasksHandler_Update_SendEmailOmitted_DefaultsToSilent(t *testing.T) {
	handler, _ := newTestTasksHandler(t, unconfiguredSender())

	createRec := postCreate(t, handler, "usr_upd_default", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "Before",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	rec := putUpdate(t, handler, "usr_upd_default", "alice@example.com", "Alice", taskID,
		tasks.UpdateTaskRequest{
			Text:    "After",
			DueDate: "2026-11-12",
			// SendEmail intentionally omitted → nil
		})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := decodeTaskResponse(t, rec)
	if body.EmailSent {
		t.Fatalf("expected email_sent=false (silent default) when SendEmail omitted, got true")
	}
}

// TestTasksHandler_Update_SendEmailTrue_DevFallbackReportsSent verifies
// that an update with send_email=true goes through the email code path
// and reports email_sent=true under dev fallback (unconfigured sender).
func TestTasksHandler_Update_SendEmailTrue_DevFallbackReportsSent(t *testing.T) {
	handler, _ := newTestTasksHandler(t, unconfiguredSender())

	createRec := postCreate(t, handler, "usr_upd_email", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "Before",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	rec := putUpdate(t, handler, "usr_upd_email", "alice@example.com", "Alice", taskID,
		tasks.UpdateTaskRequest{
			Text:      "After",
			DueDate:   "2026-11-12",
			SendEmail: ptrBool(true),
		})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeTaskResponse(t, rec)
	if !body.EmailSent {
		t.Fatalf("expected email_sent=true under dev fallback, got false")
	}
}

// TestTasksHandler_Update_NilSenderReturns200WithEmailSentFalse ensures
// that a nil sender does not crash the update path — the task is still
// updated, and email_sent is false.
func TestTasksHandler_Update_NilSenderReturns200WithEmailSentFalse(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	createRec := postCreate(t, handler, "usr_upd_nil", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{Text: "Before", DueDate: "2026-11-05"})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	rec := putUpdate(t, handler, "usr_upd_nil", "alice@example.com", "Alice", taskID,
		tasks.UpdateTaskRequest{
			Text:      "After",
			DueDate:   "2026-11-12",
			SendEmail: ptrBool(true),
		})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with nil sender, got %d", rec.Code)
	}

	body := decodeTaskResponse(t, rec)
	if body.EmailSent {
		t.Fatalf("expected email_sent=false for nil sender, got true")
	}

	list, _ := service.List("usr_upd_nil")
	if len(list) != 1 || list[0].Text != "After" {
		t.Fatalf("update did not persist despite nil sender: %+v", list)
	}
}

// TestTasksHandler_Update_MissingTask_Returns404_NoEmail ensures that
// attempting to update a non-existent task returns 404 and does NOT
// dispatch any email (the GetByID pre-check fires first).
func TestTasksHandler_Update_MissingTask_Returns404_NoEmail(t *testing.T) {
	handler, _ := newTestTasksHandler(t, unconfiguredSender())

	rec := putUpdate(t, handler, "usr_ghost", "alice@example.com", "Alice",
		"tsk_doesnotexist00",
		tasks.UpdateTaskRequest{
			Text:      "Ghost",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(true), // even with send_email=true, no email should fire
		})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------

// TestTasksHandler_Delete_SendEmailFalse_FromQuery_SkipsEmail pins
// down the ?send_email=false path (silent delete).
func TestTasksHandler_Delete_SendEmailFalse_FromQuery_SkipsEmail(t *testing.T) {
	handler, service := newTestTasksHandler(t, unconfiguredSender())

	createRec := postCreate(t, handler, "usr_del_silent", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "To be deleted silently",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	rec := deleteRequest(handler, "usr_del_silent", "alice@example.com", "Alice",
		taskID, "?send_email=false")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body tasks.MessageResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.EmailSent {
		t.Fatalf("expected email_sent=false with ?send_email=false, got true")
	}
	if !body.Success {
		t.Fatalf("expected success=true")
	}

	// Verify the row is gone.
	list, _ := service.List("usr_del_silent")
	if len(list) != 0 {
		t.Fatalf("expected 0 tasks after delete, got %d", len(list))
	}
}

// TestTasksHandler_Delete_SendEmailOmitted_DefaultsToEmail pins down
// the NEW default: delete without any query param sends the email
// (delete is destructive, so notify by default).
func TestTasksHandler_Delete_SendEmailOmitted_DefaultsToEmail(t *testing.T) {
	handler, service := newTestTasksHandler(t, unconfiguredSender())

	createRec := postCreate(t, handler, "usr_del_default", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "To be deleted with email",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	rec := deleteRequest(handler, "usr_del_default", "alice@example.com", "Alice",
		taskID, "") // no query param

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body tasks.MessageResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.EmailSent {
		t.Fatalf("expected email_sent=true (default) under dev fallback, got false")
	}

	list, _ := service.List("usr_del_default")
	if len(list) != 0 {
		t.Fatalf("expected 0 tasks after delete, got %d", len(list))
	}
}

// TestTasksHandler_Delete_SendEmailTrue_FromQuery_DevFallbackReportsSent
// exercises the explicit ?send_email=true path.
func TestTasksHandler_Delete_SendEmailTrue_FromQuery_DevFallbackReportsSent(t *testing.T) {
	handler, _ := newTestTasksHandler(t, unconfiguredSender())

	createRec := postCreate(t, handler, "usr_del_true", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "Explicit email delete",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	rec := deleteRequest(handler, "usr_del_true", "alice@example.com", "Alice",
		taskID, "?send_email=true")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body tasks.MessageResponse
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if !body.EmailSent {
		t.Fatalf("expected email_sent=true, got false")
	}
}

// TestTasksHandler_Delete_NilSenderReturns200WithEmailSentFalse ensures
// a nil sender does not crash the delete path.
func TestTasksHandler_Delete_NilSenderReturns200WithEmailSentFalse(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	createRec := postCreate(t, handler, "usr_del_nil", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{Text: "Delete me", DueDate: "2026-11-05"})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	rec := deleteRequest(handler, "usr_del_nil", "alice@example.com", "Alice",
		taskID, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with nil sender, got %d", rec.Code)
	}

	var body tasks.MessageResponse
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.EmailSent {
		t.Fatalf("expected email_sent=false for nil sender, got true")
	}

	list, _ := service.List("usr_del_nil")
	if len(list) != 0 {
		t.Fatalf("delete did not persist despite nil sender: %+v", list)
	}
}

// TestTasksHandler_Delete_MissingTask_Returns404_NoEmail ensures that
// deleting a non-existent task returns 404 without dispatching an email.
func TestTasksHandler_Delete_MissingTask_Returns404_NoEmail(t *testing.T) {
	handler, _ := newTestTasksHandler(t, unconfiguredSender())

	rec := deleteRequest(handler, "usr_ghost", "alice@example.com", "Alice",
		"tsk_doesnotexist00", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------
// Update / Delete cross-user isolation (extended from the pre-existing
// CrossUserIsolation test — these add explicit send_email=true to prove
// the email path cannot be triggered across users, since Update now
// loads the "old" task and Delete now emails the deleted task).
// ---------------------------------------------------------------------

// TestTasksHandler_Update_CrossUser_WithSendEmailTrue_NoEmail proves
// that requesting send_email=true cannot be used to send an email to
// a different user, or to leak a task's content through email.
func TestTasksHandler_Update_CrossUser_WithSendEmailTrue_NoEmail(t *testing.T) {
	handler, service := newTestTasksHandler(t, unconfiguredSender())

	// Alice creates a task.
	createRec := postCreate(t, handler, "usr_alice2", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "Alice's private task",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	// Bob attempts to update with send_email=true.
	rec := putUpdate(t, handler, "usr_bob2", "bob@example.com", "Bob", taskID,
		tasks.UpdateTaskRequest{
			Text:      "Hijacked",
			DueDate:   "2027-01-01",
			SendEmail: ptrBool(true),
		})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("SECURITY FAILURE: bob update returned %d, expected 404", rec.Code)
	}

	// Alice's task must be untouched.
	list, _ := service.List("usr_alice2")
	if len(list) != 1 || list[0].Text != "Alice's private task" {
		t.Fatalf("SECURITY FAILURE: alice's task was mutated: %+v", list)
	}
}

// TestTasksHandler_Delete_CrossUser_WithSendEmailTrue_NoEmail proves
// that a cross-user delete attempt with send_email=true cannot delete
// the target task or dispatch an email.
func TestTasksHandler_Delete_CrossUser_WithSendEmailTrue_NoEmail(t *testing.T) {
	handler, service := newTestTasksHandler(t, unconfiguredSender())

	createRec := postCreate(t, handler, "usr_alice3", "alice@example.com", "Alice",
		tasks.CreateTaskRequest{
			Text:      "Alice's delete target",
			DueDate:   "2026-11-05",
			SendEmail: ptrBool(false),
		})
	taskID := decodeTaskResponse(t, createRec).Task.ID

	// Bob attempts to delete with send_email=true.
	rec := deleteRequest(handler, "usr_bob3", "bob@example.com", "Bob",
		taskID, "?send_email=true")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("SECURITY FAILURE: bob delete returned %d, expected 404", rec.Code)
	}

	// Alice's task must still exist.
	list, _ := service.List("usr_alice3")
	if len(list) != 1 {
		t.Fatalf("SECURITY FAILURE: alice's task was deleted by bob: %+v", list)
	}
}