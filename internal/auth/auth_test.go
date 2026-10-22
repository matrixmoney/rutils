package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"Valid password with exclamation", "Password123!", false},
		{"Valid password with hash", "Secret#99xyz", false},
		{"Valid password with various symbols", "Str0ng@Work$", false},
		{"Too short", "P@1a", true},
		{"Missing uppercase", "password123!", true},
		{"Missing lowercase", "PASSWORD123!", true},
		{"Missing number", "Password!!!!", true},
		{"Missing special character", "Password1234", true},
		{"Empty password", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestBcryptPasswordHashing(t *testing.T) {
	raw := "SecureP@ssw0rd2026!"
	hash, err := HashPassword(raw)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !ComparePassword(hash, raw) {
		t.Fatalf("ComparePassword failed for valid password")
	}

	if ComparePassword(hash, "WrongP@ssw0rd!") {
		t.Fatalf("ComparePassword succeeded for invalid password")
	}
}

func TestGenerateTemporaryPassword(t *testing.T) {
	for i := 0; i < 20; i++ {
		temp := GenerateTemporaryPassword()
		if len(temp) != 14 {
			t.Fatalf("expected length 14, got %d for %q", len(temp), temp)
		}
		if err := ValidatePassword(temp); err != nil {
			t.Fatalf("GenerateTemporaryPassword produced invalid password %q: %v", temp, err)
		}
	}
}

func TestJWTGenerationAndValidation(t *testing.T) {
	cfg := &Config{
		JWTSecret:          "test-super-secret-key-1234567890",
		JWTExpirationHours: 2,
	}

	// ⬅️ CHANGED: GenerateToken now takes a `name` argument (5 args total).
	tokenStr, err := GenerateToken(cfg, "user_123", "alice@example.com", "Alice Smith", false)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, err := ValidateToken(cfg, tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != "user_123" {
		t.Errorf("expected userID 'user_123', got %q", claims.UserID)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("expected email 'alice@example.com', got %q", claims.Email)
	}
	// ⬅️ NEW: assert the name round-trips through the signed token, since the
	// tasks handler relies on claims.Name to personalise outgoing emails.
	if claims.Name != "Alice Smith" {
		t.Errorf("expected name 'Alice Smith', got %q", claims.Name)
	}
	if claims.MustResetPassword != false {
		t.Errorf("expected mustReset false, got true")
	}

	// Validate with wrong secret
	wrongCfg := &Config{
		JWTSecret: "wrong-secret-key",
	}
	_, err = ValidateToken(wrongCfg, tokenStr)
	if err == nil {
		t.Fatalf("expected validation failure with wrong secret, got nil")
	}
}

func TestStoreCRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "users.json")
	store, err := NewStore(filePath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Create user
	u := &User{
		Name:         "Alice Smith",
		Email:        "Alice@Example.COM",
		PasswordHash: "hashed_pwd",
	}
	if err := store.Create(u); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	if u.ID == "" {
		t.Fatalf("expected generated user ID, got empty")
	}

	// Case-insensitive lookup
	found, err := store.GetByEmail("alice@example.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}
	if found.Name != "Alice Smith" {
		t.Errorf("expected name 'Alice Smith', got %q", found.Name)
	}

	// Lookup by ID
	foundByID, err := store.GetByID(u.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if foundByID.Email != "alice@example.com" {
		t.Errorf("expected normalized email 'alice@example.com', got %q", foundByID.Email)
	}

	// Duplicate email rejection
	dup := &User{
		Name:         "Duplicate Alice",
		Email:        "ALICE@example.com",
		PasswordHash: "hashed_pwd2",
	}
	if err := store.Create(dup); err != ErrEmailAlreadyExists {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}

	// Update user
	found.Name = "Alice Johnson"
	found.MustResetPassword = true
	now := time.Now()
	found.TempPasswordExpiry = &now
	if err := store.Update(found); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := store.GetByID(found.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if updated.Name != "Alice Johnson" {
		t.Errorf("expected name 'Alice Johnson', got %q", updated.Name)
	}
	if !updated.MustResetPassword {
		t.Errorf("expected MustResetPassword true")
	}
}
