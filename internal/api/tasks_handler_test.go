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
//
// This lets us exercise the handler in isolation — no JWT parsing, no
// middleware wiring, no HTTP server. If we wanted to test the full
// stack (RequireAuth + handler together) we would spin up a real
// server, which is what cmd/server/auth_api_test.go already does for
// the auth endpoints.
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

// ============================================================
// Test 1: the nil-sender contract (the headline)
// ============================================================

// TestTasksHandler_Create_WithNilEmailSender pins down the single most
// important guarantee of the Tasks feature:
//
//	When the handler is constructed with a nil *auth.EmailSender,
//	POST /tasks MUST still succeed, MUST still save the task, and
//	MUST return email_sent=false. It MUST NOT panic.
//
// This protects against a future refactor that:
//   - removes the `if h.emailSender == nil { return false }` guard
//     in dispatchTaskEmail, or
//   - adds a new code path that dereferences h.emailSender before
//     the nil check.
//
// If that ever happens, this test panics out with a clear stack
// trace pointing at the nil dereference — much easier to diagnose
// than a random 500 in production.
func TestTasksHandler_Create_WithNilEmailSender(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	// Sanity: confirm the handler really is nil-sender, otherwise this
	// test would silently stop testing the nil path after a refactor.
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

	// The contract: this must not panic. If it does, the test fails with
	// a stack trace pointing at the offending line.
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
	if body.Task.Text != "Buy groceries for the weekend" {
		t.Fatalf("expected task text to round-trip, got %q", body.Task.Text)
	}
	if body.Task.DueDate != "2026-11-05" {
		t.Fatalf("expected due date to round-trip, got %q", body.Task.DueDate)
	}

	// Verify the task actually persisted, not just echoed in the response.
	list, err := service.List()
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 task in the store, got %d", len(list))
	}
	if list[0].ID != body.Task.ID {
		t.Fatalf("persisted task ID %q does not match response ID %q", list[0].ID, body.Task.ID)
	}
}

// ============================================================
// Test 2: the dev-fallback contract
// ============================================================

// TestTasksHandler_Create_EmailSenderWithoutConfiguredSMTP pins down
// the dev-mode guarantee:
//
//	When the EmailSender is present but SMTP is unconfigured
//	(empty SMTPEmail / SMTPAppPassword), SendTaskNotification falls
//	back to logging and returns nil. Therefore POST /tasks MUST
//	return email_sent=true — because the dispatch "succeeded" in
//	the sense that the handler has nothing more it could do.
//
// If someone later "fixes" the dev fallback to return an error
// (which would be a UX regression for local development), this test
// fails and forces them to reconsider.
//
// Nothing is sent over the network: the fallback path returns before
// touching smtp.SendMail, so this test runs offline.
func TestTasksHandler_Create_EmailSenderWithoutConfiguredSMTP(t *testing.T) {
	// Build a real EmailSender with SMTP intentionally unconfigured.
	// The default Config has empty SMTPEmail/SMTPAppPassword, so
	// IsConfigured() returns false.
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

	// In dev fallback, the notification "succeeded" — the fallback
	// logs and returns nil, so EmailSent must be true.
	if !body.EmailSent {
		t.Fatalf("expected email_sent=true under dev fallback, got false")
	}

	// Confirm persistence.
	list, err := service.List()
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 task in the store, got %d", len(list))
	}
}

// ============================================================
// Test 3: handler-level validation mapping
// ============================================================

// TestTasksHandler_Create_RejectsInvalidPayload checks that the
// handler maps a service-level validation error (empty text) to a
// 400 Bad Request with a JSON error envelope.
//
// This is already indirectly covered by the tasks service tests, but
// asserting it at the handler level confirms the HTTP status mapping
// is what API clients expect — the two concerns are separable and
// worth testing separately.
func TestTasksHandler_Create_RejectsInvalidPayload(t *testing.T) {
	handler, service := newTestTasksHandler(t, nil)

	payload, _ := json.Marshal(tasks.CreateTaskRequest{
		Text:    "   ", // whitespace-only → rejected by service
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

	// Confirm nothing was persisted.
	list, err := service.List()
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 tasks in the store, got %d", len(list))
	}
}