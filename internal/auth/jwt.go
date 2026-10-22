package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims defines custom JWT claims
//
// ⬅️ CHANGED: added `Name` so downstream handlers (e.g. the tasks handler)
// can personalise outgoing emails ("Hello Jane,") without an extra
// user-store lookup. Older tokens issued before this field existed
// decode with Name == "", which is safe — consumers fall back to a
// generic greeting.
type Claims struct {
	UserID            string `json:"user_id"`
	Email             string `json:"email"`
	Name              string `json:"name"` // ⬅️ NEW
	MustResetPassword bool   `json:"must_reset_password"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT string for a user
//
// ⬅️ CHANGED: signature now includes `name string`.
//
// Callers must pass the user's display name (from User.Name). The name
// is embedded in the token's `name` claim and is read back by handlers
// via api.GetUserClaims(r.Context()).Name.
func GenerateToken(cfg *Config, userID, email, name string, mustReset bool) (string, error) {
	if cfg.JWTSecret == "" {
		return "", errors.New("jwt secret is not configured")
	}

	expirationTime := time.Now().Add(time.Duration(cfg.JWTExpirationHours) * time.Hour)
	claims := &Claims{
		UserID:            userID,
		Email:             email,
		Name:              name, // ⬅️ NEW
		MustResetPassword: mustReset,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "rutils-auth",
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken parses and validates a JWT token string
//
// UNCHANGED: the decoder populates the whole Claims struct, so the new
// `Name` field is read back automatically when present. Tokens issued
// before this change decode with Name == "" — safe for every caller.
func ValidateToken(cfg *Config, tokenStr string) (*Claims, error) {
	if cfg.JWTSecret == "" {
		return nil, errors.New("jwt secret is not configured")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid or expired token")
	}

	return claims, nil
}