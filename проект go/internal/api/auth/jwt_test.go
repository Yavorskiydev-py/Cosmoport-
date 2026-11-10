package auth

import (
	"errors"
	"testing"
	"time"
)

func TestJWTManager_GenerateAndParse(t *testing.T) {
	m := NewJWTManager("secret", time.Hour)

	token, exp, err := m.Generate(42)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if time.Until(exp) <= 0 {
		t.Fatalf("expiresAt must be in the future")
	}

	userID, err := m.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if userID != 42 {
		t.Fatalf("want user 42, got %d", userID)
	}
}

func TestJWTManager_RejectsInvalidTokens(t *testing.T) {
	m := NewJWTManager("secret", time.Hour)
	other := NewJWTManager("another-secret", time.Hour)
	expired := NewJWTManager("secret", -time.Minute)

	foreign, _, _ := other.Generate(1)
	old, _, _ := expired.Generate(1)

	for name, token := range map[string]string{
		"garbage":      "not-a-token",
		"wrong secret": foreign,
		"expired":      old,
		"empty":        "",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := m.Parse(token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("want ErrInvalidToken, got %v", err)
			}
		})
	}
}
