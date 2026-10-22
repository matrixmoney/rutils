package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"file-converter/internal/auth"
	"file-converter/internal/nifty"
)

func generateTestCompanies(count int) []nifty.Company {
	var companies []nifty.Company
	for i := 1; i <= count; i++ {
		companies = append(companies, nifty.Company{
			CompanyName: fmt.Sprintf("Company %d", i),
			Industry:    fmt.Sprintf("Sector %d", (i%5)+1),
			Symbol:      fmt.Sprintf("SYM%d", i),
			Series:      "EQ",
			ISIN:        fmt.Sprintf("INE%09d", i),
		})
	}
	return companies
}

func setupTestServer(t *testing.T, dataFile string) (http.Handler, *auth.Config, string) {
	t.Helper()

	cfg := auth.DefaultConfig()
	cfg.JWTSecret = "test-jwt-secret-key-for-testing-12345"

	storage := nifty.NewFileStorage(dataFile)
	client := nifty.NewNSEClient()
	service := nifty.NewService(storage, client)
	handler := NewNiftyHandler(service)

	mux := http.NewServeMux()
	mux.HandleFunc("/nifty50/companies", RequireAuth(cfg, handler.GetCompanies))

	// Generate a valid token
		// ⬅️ CHANGED: GenerateToken now takes a `name` argument (5 args total).
	token, err := auth.GenerateToken(cfg, "user-123", "test@example.com", "Test User", false)
	if err != nil {
		t.Fatalf("failed to generate test JWT: %v", err)
	}

	return mux, cfg, token
}

func TestNiftyHandler_Unauthenticated(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "nifty50.json")

	handler, _, _ := setupTestServer(t, dataFile)

	// Request without Authorization header
	req := httptest.NewRequest(http.MethodGet, "/nifty50/companies", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", rr.Code)
	}

	// Request with invalid token
	req = httptest.NewRequest(http.MethodGet, "/nifty50/companies", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized with bad token, got %d", rr.Code)
	}
}

func TestNiftyHandler_DatasetNotAvailable_503(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "non_existent_nifty50.json")

	handler, _, token := setupTestServer(t, dataFile)

	req := httptest.NewRequest(http.MethodGet, "/nifty50/companies", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 Service Unavailable when data missing, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["success"] != false {
		t.Errorf("expected success: false in 503 response, got %v", resp["success"])
	}
	if !strings.Contains(fmt.Sprint(resp["error"]), "unavailable") {
		t.Errorf("expected unavailable message in error, got %v", resp["error"])
	}
}

func TestNiftyHandler_Authenticated_Success_200(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "nifty50.json")

	// Pre-populate data file
	storage := nifty.NewFileStorage(dataFile)
	testCompanies := generateTestCompanies(50)
	timestamp := time.Now().Format(time.RFC3339)
	err := storage.Save(&nifty.NiftyData{
		UpdatedAt: timestamp,
		Count:     50,
		Companies: testCompanies,
	})
	if err != nil {
		t.Fatalf("failed to save test data: %v", err)
	}

	handler, _, token := setupTestServer(t, dataFile)

	req := httptest.NewRequest(http.MethodGet, "/nifty50/companies", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp nifty.CompaniesResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode 200 response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success: true, got %v", resp.Success)
	}
	if resp.Count != 50 {
		t.Errorf("expected count: 50, got %d", resp.Count)
	}
	if len(resp.Companies) != 50 {
		t.Errorf("expected 50 companies, got %d", len(resp.Companies))
	}
	if resp.UpdatedAt != timestamp {
		t.Errorf("expected updated_at %q, got %q", timestamp, resp.UpdatedAt)
	}

	first := resp.Companies[0]
	if first.CompanyName != "Company 1" || first.Symbol != "SYM1" || first.Series != "EQ" || first.ISIN != "INE000000001" {
		t.Errorf("first company content mismatch: %+v", first)
	}
}
