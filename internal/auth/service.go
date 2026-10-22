package auth

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// Common authentication errors
var (
	ErrInvalidCredentials       = errors.New("invalid email or password")
	ErrPasswordMismatch         = errors.New("passwords do not match")
	ErrInvalidEmail             = errors.New("invalid email address format")
	ErrCurrentPasswordInvalid   = errors.New("current password does not match")
	ErrTemporaryPasswordExpired = errors.New("temporary password has expired, please request a new one")
	ErrPasswordResetPending     = errors.New("a password reset was requested. Please log in using the temporary password sent to your email")
)

// AuthService encapsulates authentication logic
type AuthService struct {
	store       *Store
	cfg         *Config
	emailSender *EmailSender
}

// NewAuthService creates a new instance of AuthService
func NewAuthService(store *Store, cfg *Config, emailSender *EmailSender) *AuthService {
	return &AuthService{
		store:       store,
		cfg:         cfg,
		emailSender: emailSender,
	}
}

// Register registers a new user account with strong password requirements
func (s *AuthService) Register(req RegisterRequest) (*UserResponse, error) {
	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(req.Email)

	if name == "" {
		return nil, errors.New("name is required")
	}
	if email == "" {
		return nil, errors.New("email is required")
	}
	if _, err := mail.ParseAddress(email); err != nil || !strings.Contains(email, ".") {
		return nil, ErrInvalidEmail
	}
	if req.Password == "" {
		return nil, errors.New("password is required")
	}
	if req.Password != req.ConfirmPassword {
		return nil, ErrPasswordMismatch
	}

	if err := ValidatePassword(req.Password); err != nil {
		return nil, err
	}

	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &User{
		Name:              name,
		Email:             email,
		PasswordHash:      passwordHash,
		MustResetPassword: false,
	}

	if err := s.store.Create(user); err != nil {
		return nil, err
	}

	resp := user.ToResponse()
	return &resp, nil
}

// Login authenticates a user with either permanent or temporary password and issues a JWT token
func (s *AuthService) Login(req LoginRequest) (*LoginResponse, error) {
	email := strings.TrimSpace(req.Email)
	if email == "" || req.Password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := s.store.GetByEmail(email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// 1. Check if temporary password was used
	if user.TempPasswordHash != "" && ComparePassword(user.TempPasswordHash, req.Password) {
		if user.TempPasswordExpiry != nil && time.Now().After(*user.TempPasswordExpiry) {
			return nil, ErrTemporaryPasswordExpired
		}

		// User authenticated with temporary password; force password reset
		token, err := GenerateToken(s.cfg, user.ID, user.Email, user.Name, true) // ⬅️ CHANGED
		if err != nil {
			return nil, fmt.Errorf("failed to generate authentication token: %w", err)
		}

		return &LoginResponse{
			Token:             token,
			User:              user.ToResponse(),
			MustResetPassword: true,
			Message:           "Logged in with temporary password. You must set a new permanent password.",
		}, nil
	}

	// 2. If a temporary password has been issued or password reset is required,
	// the old permanent password cannot be used to log in.
	if user.TempPasswordHash != "" || user.MustResetPassword {
		if user.TempPasswordExpiry != nil && time.Now().After(*user.TempPasswordExpiry) {
			return nil, ErrTemporaryPasswordExpired
		}
		if ComparePassword(user.PasswordHash, req.Password) {
			return nil, ErrPasswordResetPending
		}
		return nil, ErrInvalidCredentials
	}

	// 3. Check permanent password
	if !ComparePassword(user.PasswordHash, req.Password) {
		return nil, ErrInvalidCredentials
	}

	token, err := GenerateToken(s.cfg, user.ID, user.Email, user.Name, false) // ⬅️ CHANGED
	if err != nil {
		return nil, fmt.Errorf("failed to generate authentication token: %w", err)
	}

	return &LoginResponse{
		Token:             token,
		User:              user.ToResponse(),
		MustResetPassword: false,
		Message:           "Login successful",
	}, nil
}

// ForgotPassword generates a temporary strong password and sends it via Google SMTP
func (s *AuthService) ForgotPassword(req ForgotPasswordRequest) error {
	email := strings.TrimSpace(req.Email)
	if email == "" {
		return errors.New("email is required")
	}

	user, err := s.store.GetByEmail(email)
	if err != nil {
		return ErrUserNotFound
	}

	tempPassword := GenerateTemporaryPassword()
	tempHash, err := HashPassword(tempPassword)
	if err != nil {
		return fmt.Errorf("failed to hash temporary password: %w", err)
	}

	expiry := time.Now().Add(24 * time.Hour)
	user.TempPasswordHash = tempHash
	user.TempPasswordExpiry = &expiry
	user.MustResetPassword = true

	if err := s.store.Update(user); err != nil {
		return fmt.Errorf("failed to update user with temporary password: %w", err)
	}

	if err := s.emailSender.SendTemporaryPassword(user.Email, user.Name, tempPassword); err != nil {
		return fmt.Errorf("failed to deliver temporary password email: %w", err)
	}

	return nil
}

// ResetPassword sets a permanent password after logging in with a temporary password
func (s *AuthService) ResetPassword(userID string, req ResetPasswordRequest) error {
	if req.NewPassword == "" {
		return errors.New("new password is required")
	}
	if req.NewPassword != req.ConfirmPassword {
		return ErrPasswordMismatch
	}

	if err := ValidatePassword(req.NewPassword); err != nil {
		return err
	}

	user, err := s.store.GetByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	user.PasswordHash = newHash
	user.TempPasswordHash = ""
	user.TempPasswordExpiry = nil
	user.MustResetPassword = false

	if err := s.store.Update(user); err != nil {
		return fmt.Errorf("failed to update user password: %w", err)
	}

	return nil
}

// ChangePassword changes the password for an already logged-in user and sends an email notification
func (s *AuthService) ChangePassword(userID string, req ChangePasswordRequest) error {
	if req.CurrentPassword == "" {
		return errors.New("current password is required")
	}
	if req.NewPassword == "" {
		return errors.New("new password is required")
	}
	if req.NewPassword != req.ConfirmPassword {
		return ErrPasswordMismatch
	}

	if err := ValidatePassword(req.NewPassword); err != nil {
		return err
	}

	user, err := s.store.GetByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	if !ComparePassword(user.PasswordHash, req.CurrentPassword) {
		return ErrCurrentPasswordInvalid
	}

	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	user.PasswordHash = newHash
	user.TempPasswordHash = ""
	user.TempPasswordExpiry = nil
	user.MustResetPassword = false

	if err := s.store.Update(user); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Send confirmation email
	_ = s.emailSender.SendPasswordChangedNotification(user.Email, user.Name)

	return nil
}

// GetUserProfile retrieves the user's public profile
func (s *AuthService) GetUserProfile(userID string) (*UserResponse, error) {
	user, err := s.store.GetByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	resp := user.ToResponse()
	return &resp, nil
}
