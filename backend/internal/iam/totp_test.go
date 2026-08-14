package iam

import (
	"strings"
	"testing"
	"time"
)

func TestTOTPSecretGeneration(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	if secret == "" {
		t.Fatal("secret must not be empty")
	}
	// 20 random bytes encode to 32 base32 characters.
	if len(secret) != 32 {
		t.Errorf("expected base32 secret length 32, got %d", len(secret))
	}
	if strings.ToUpper(secret) != secret {
		t.Error("secret should be uppercase base32")
	}

	secret2, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate second secret: %v", err)
	}
	if secret == secret2 {
		t.Error("two generated secrets must be distinct")
	}
}

func TestTOTPCodeDeterministic(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}

	now := time.Unix(1000000000, 0)
	code1, err := TOTPCode(secret, now)
	if err != nil {
		t.Fatalf("compute code: %v", err)
	}
	if len(code1) != TOTPDigits {
		t.Errorf("expected %d digit code, got %s", TOTPDigits, code1)
	}

	code2, err := TOTPCode(secret, now)
	if err != nil {
		t.Fatalf("compute code again: %v", err)
	}
	if code1 != code2 {
		t.Error("code must be deterministic for the same time step")
	}

	otherSecret, _ := GenerateTOTPSecret()
	otherCode, _ := TOTPCode(otherSecret, now)
	if code1 == otherCode {
		t.Error("different secrets must produce different codes")
	}
}

func TestTOTPVerifyValidAndInvalidCodes(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}

	now := time.Now().UTC()
	code, err := TOTPCode(secret, now)
	if err != nil {
		t.Fatalf("compute code: %v", err)
	}

	ok, err := VerifyTOTP(secret, code, now, TOTPSkew)
	if err != nil {
		t.Fatalf("verify current code: %v", err)
	}
	if !ok {
		t.Error("current code must verify")
	}

	ok, err = VerifyTOTP(secret, "000000", now, TOTPSkew)
	if err != nil {
		t.Fatalf("verify wrong code: %v", err)
	}
	if ok {
		t.Error("wrong code must not verify")
	}

	ok, err = VerifyTOTP(secret, "12345", now, TOTPSkew)
	if err != nil {
		t.Fatalf("verify short code: %v", err)
	}
	if ok {
		t.Error("short code must not verify")
	}
}

func TestTOTPVerifySkew(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}

	now := time.Now().UTC()
	past := now.Add(-TOTPPeriod)
	code, err := TOTPCode(secret, past)
	if err != nil {
		t.Fatalf("compute past code: %v", err)
	}

	ok, err := VerifyTOTP(secret, code, now, TOTPSkew)
	if err != nil {
		t.Fatalf("verify past code with skew: %v", err)
	}
	if !ok {
		t.Error("code one period in the past must verify with skew=1")
	}

	future := now.Add(TOTPPeriod)
	code, err = TOTPCode(secret, future)
	if err != nil {
		t.Fatalf("compute future code: %v", err)
	}

	ok, err = VerifyTOTP(secret, code, now, TOTPSkew)
	if err != nil {
		t.Fatalf("verify future code with skew: %v", err)
	}
	if !ok {
		t.Error("code one period in the future must verify with skew=1")
	}

	// Three periods away is outside the accepted skew window.
	stale := now.Add(-3 * TOTPPeriod)
	code, err = TOTPCode(secret, stale)
	if err != nil {
		t.Fatalf("compute stale code: %v", err)
	}

	ok, err = VerifyTOTP(secret, code, now, TOTPSkew)
	if err != nil {
		t.Fatalf("verify stale code: %v", err)
	}
	if ok {
		t.Error("code three periods away must not verify with skew=1")
	}
}
