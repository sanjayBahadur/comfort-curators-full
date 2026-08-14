package iam

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

const (
	DefaultOTPLength = 6
	DefaultOTPTTL    = 5 * time.Minute
	DefaultOTPMaxAge = 1 * time.Minute
)

func GenerateOTP(length int) (*OTP, error) {
	if length <= 0 {
		length = DefaultOTPLength
	}

	token := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(16))
		if err != nil {
			return nil, fmt.Errorf("generate otp: %w", err)
		}
		token[i] = "0123456789abcdef"[n.Int64()]
	}

	return &OTP{
		Token:     string(token),
		ExpiresAt: time.Now().UTC().Add(DefaultOTPTTL),
	}, nil
}

func HashOTP(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func VerifyOTPHash(token string, hash string) bool {
	return HashOTP(token) == hash
}

func GenerateSessionToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
