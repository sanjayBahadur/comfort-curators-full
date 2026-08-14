package iam

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// TOTPDigits is the number of digits in a TOTP code.
	TOTPDigits = 6
	// TOTPPeriod is the time-step size defined by RFC 6238.
	TOTPPeriod = 30 * time.Second
	// TOTPSkew is the number of periods on either side of the current
	// time step that are accepted when verifying a code. A skew of 1
	// allows roughly +/- 30 seconds of clock skew.
	TOTPSkew = 1
	// TOTPSecretBytes is the length of the random seed used to generate
	// a TOTP secret. 20 bytes (160 bits) is the default recommended by
	// RFC 4226/RFC 6238 and produces a 32-character base32 string.
	TOTPSecretBytes = 20
)

// GenerateTOTPSecret returns a new base32-encoded TOTP secret suitable for
// provisioning into an authenticator application.
func GenerateTOTPSecret() (string, error) {
	var b [TOTPSecretBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return base32.StdEncoding.EncodeToString(b[:]), nil
}

// TOTPProvisioningURI builds an otpauth:// URI that authenticator apps can
// consume. The label is typically the user's contact address and the issuer
// is the application name.
func TOTPProvisioningURI(secret, label, issuer string) string {
	path := "/" + issuer + ":" + label
	u := url.URL{
		Scheme: "otpauth",
		Host:   "totp",
		Path:   path,
	}
	q := u.Query()
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	u.RawQuery = q.Encode()
	return u.String()
}

// TOTPCode computes the RFC 6238 TOTP code for the given secret and time.
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := base32.StdEncoding.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", fmt.Errorf("decode totp secret: %w", err)
	}

	counter := uint64(t.Unix()) / uint64(TOTPPeriod.Seconds())
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := (int(sum[offset]&0x7f)<<24 |
		int(sum[offset+1])<<16 |
		int(sum[offset+2])<<8 |
		int(sum[offset+3])) % 1000000

	return fmt.Sprintf("%0*d", TOTPDigits, code), nil
}

// VerifyTOTP verifies a user-supplied TOTP code against the secret using the
// RFC 6238 algorithm with the configured time step and skew.
func VerifyTOTP(secret, code string, t time.Time, skew int) (bool, error) {
	if len(code) != TOTPDigits {
		return false, nil
	}

	for i := -skew; i <= skew; i++ {
		step := t.Add(time.Duration(i) * TOTPPeriod)
		expected, err := TOTPCode(secret, step)
		if err != nil {
			return false, err
		}
		if hmac.Equal([]byte(code), []byte(expected)) {
			return true, nil
		}
	}

	return false, nil
}
