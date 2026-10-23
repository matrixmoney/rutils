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

func withClaimsContext(r *http.Request, userID, email, name string) *http.Request {
	claims := &auth.Claims{
		UserID: userID,
		Email:  email,
		Name:   name,
	}
	ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
	return r.WithContext(ctx)
}

func decodeTaskResponse(t *testing.T, rec *httptest.ResponseRecorder) tasks.TaskResponse {
	t.Helper()

	var body tasks.TaskResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v (raw: %q)", err, rec.Body.String())
	}
	return body
}

func TestTasksHandler_Create_WithNilEmailSender(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	if handler.emailSender != nil {
		t.Fatalf("test setup error: handler.emailSender must be nil for this test")
	}

	payload, _ := json.Marshal(tasks.CreateTaskRequest{
		Text:    "Buy groceries for the weekend",
		DueDate: "2026-11-05",
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = withClaimsContext(req, "usr_test123", "alice@example.com", "Alice Smith")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

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
		t.Fatalf("expected the task to be returned with a generated ID, got empty")
	}
	if body.Task.UserID != "usr_test123" {
		t.Fatalf("expected task.user_id=usr_test123, got %q", body.Task.UserID)
	}
	if body.Task.Text != "Buy groceries for the weekend" {
		t.Fatalf("expected task text to round-trip, got %q", body.Task.Text)
	}
	if body.Task.DueDate != "2026-11-05" {
		t.Fatalf("expected due date to round-trip, got %q", body.Task.DueDate)
	}

	list, err := service.List("usr_test123")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 task for owner, got %d", len(list))
	}
	if list[0].ID != body.Task.ID {
		t.Fatalf("persisted task ID %q does not match response ID %q", list[0].ID, body.Task.ID)
	}
}

func TestTasksHandler_Create_EmailSenderWithoutConfiguredSMTP(t *testing.T) {
	cfg := auth.DefaultConfig()
	sender := auth.NewEmailSender(cfg)

	if sender.IsConfigured() {
		t.Fatalf("test setup error: sender must not be configured for this test")
	}

	handler, service := newTestTasksHandler(t, sender)

	payload, _ := json.Marshal(tasks.CreateTaskRequest{
		Text:    "Water the plants",
		DueDate: "2026-10-30",
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = withClaimsContext(req, "usr_test456", "bob@example.com", "Bob Jones")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeTaskResponse(t, rec)

	if !body.Success {
		t.Fatalf("expected success=true, got %v", body.Success)
	}
	if !body.EmailSent {
		t.Fatalf("expected email_sent=true under dev fallback, got false")
	}
	if body.Task.UserID != "usr_test456" {
		t.Fatalf("expected task.user_id=usr_test456, got %q", body.Task.UserID)
	}

	list, err := service.List("usr_test456")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 task for owner, got %d", len(list))
	}
}

func TestTasksHandler_Create_RejectsInvalidPayload(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	payload, _ := json.Marshal(tasks.CreateTaskRequest{
		Text:    "   ",
		DueDate: "2026-11-05",
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = withClaimsContext(req, "usr_test789", "carol@example.com", "Carol White")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var errBody map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if errBody["error"] == "" {
		t.Fatalf("expected a non-empty error message, got %v", errBody)
	}

	list, err := service.List("usr_test789")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 tasks for owner, got %d", len(list))
	}
}

// TestTasksHandler_CrossUserIsolation pins down the security property:
//   - User A creates a task.
//   - User B cannot see it via List.
//   - User B cannot update it (404).
//   - User B cannot delete it (404).
//   - User A still sees it, unchanged, after all of B's failed attempts.
func TestTasksHandler_CrossUserIsolation(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	// --- Step 1: User A creates a task. ---
	createPayload, _ := json.Marshal(tasks.CreateTaskRequest{
		Text:    "Alice's private note",
		DueDate: "2026-11-05",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(createPayload))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = withClaimsContext(createReq, "usr_alice", "alice@example.com", "Alice")

	createRec := httptest.NewRecorder()
	handler.Create(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("alice create failed: %d %s", createRec.Code, createRec.Body.String())
	}
	aliceTaskID := decodeTaskResponse(t, createRec).Task.ID

	// --- Step 2: User B lists their tasks → empty. ---
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
	if !bobList.Success {
		t.Fatalf("expected bob list success=true, got false")
	}
	if len(bobList.Tasks) != 0 {
		t.Fatalf("SECURITY FAILURE: bob can see %d task(s) he does not own", len(bobList.Tasks))
	}

	// --- Step 3: User B updates Alice's task → 404. ---
	updatePayload, _ := json.Marshal(tasks.UpdateTaskRequest{
		Text:    "Bob tried to hijack this",
		DueDate: "2027-01-01",
	})
	updateReq := httptest.NewRequest(http.MethodPut, "/tasks/"+aliceTaskID, bytes.NewReader(updatePayload))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq = withClaimsContext(updateReq, "usr_bob", "bob@example.com", "Bob")

	updateRec := httptest.NewRecorder()
	handler.Update(updateRec, updateReq)
	if updateRec.Code != http.StatusNotFound {
		t.Fatalf("SECURITY FAILURE: bob update of alice's task returned %d, expected 404", updateRec.Code)
	}

	// --- Step 4: User B deletes Alice's task → 404. ---
	deleteReq := httptest.NewRequest(http.MethodDelete, "/tasks/"+aliceTaskID, nil)
	deleteReq = withClaimsContext(deleteReq, "usr_bob", "bob@example.com", "Bob")

	deleteRec := httptest.NewRecorder()
	handler.Delete(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNotFound {
		t.Fatalf("SECURITY FAILURE: bob delete of alice's task returned %d, expected 404", deleteRec.Code)
	}

	// --- Step 5: Alice's task still exists and is unchanged. ---
	aliceList, err := service.List("usr_alice")
	if err != nil {
		t.Fatalf("failed to list alice's tasks: %v", err)
	}
	if len(aliceList) != 1 {
		t.Fatalf("expected alice to still have 1 task, got %d", len(aliceList))
	}
	if aliceList[0].Text != "Alice's private note" {
		t.Fatalf("alice's task text was mutated: %q", aliceList[0].Text)
	}
	if aliceList[0].DueDate != "2026-11-05" {
		t.Fatalf("alice's due date was mutated: %q", aliceList[0].DueDate)
	}
}