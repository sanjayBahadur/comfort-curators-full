package communications

import (
	"strings"
	"testing"
	"time"
)

func TestAudienceAndConsentValidation(t *testing.T) {
	if !IsValidAudience(AudienceOwner) || !IsValidAudience(AudienceGuest) {
		t.Fatal("owner and guest must be valid audiences")
	}
	for _, bad := range []string{"", "host", "both", "OWNER"} {
		if IsValidAudience(bad) {
			t.Fatalf("audience %q must be rejected", bad)
		}
	}

	for _, class := range []string{
		ConsentClassTransactional, ConsentClassUrgent, ConsentClassMarketing, ConsentClassSponsored,
	} {
		if !IsValidConsentClass(class) {
			t.Fatalf("consent class %q must be valid", class)
		}
	}
	if IsValidConsentClass("promotional") {
		t.Fatal("unknown consent class must be rejected")
	}

	if !IsValidSource(SourceTemplate) || !IsValidSource(SourceAI) {
		t.Fatal("template and ai must be valid draft sources")
	}
	if IsValidSource("model") {
		t.Fatal("unknown draft source must be rejected")
	}

	if !IsValidLanguage(LanguageEnglish) || !IsValidLanguage(LanguageHindi) {
		t.Fatal("en and hi must be valid template languages")
	}
	if IsValidLanguage("fr") {
		t.Fatal("unknown template language must be rejected")
	}
}

func TestConsentByClass(t *testing.T) {
	prefs := defaultPreferences("t", "r", AudienceGuest)

	if !ConsentByClass(prefs, ConsentClassTransactional) {
		t.Fatal("transactional consent must default to granted")
	}
	if !ConsentByClass(prefs, ConsentClassUrgent) {
		t.Fatal("urgent consent must default to granted")
	}
	if ConsentByClass(prefs, ConsentClassMarketing) {
		t.Fatal("marketing consent must default to not granted")
	}
	if ConsentByClass(prefs, ConsentClassSponsored) {
		t.Fatal("sponsored consent must default to not granted")
	}

	prefs.ConsentMarketing = true
	if !ConsentByClass(prefs, ConsentClassMarketing) {
		t.Fatal("marketing consent must reflect the stored flag")
	}
	prefs.ConsentSponsored = false
	if ConsentByClass(prefs, ConsentClassSponsored) {
		t.Fatal("sponsored consent must remain false")
	}
}

func TestIsWithinQuietHours(t *testing.T) {
	at := func(h, m int) time.Time {
		return time.Date(2026, time.August, 5, h, m, 0, 0, time.UTC)
	}

	// Disabled window (start == end) never blocks.
	if IsWithinQuietHours(at(14, 0), 0, 0) {
		t.Fatal("zero-length quiet hours must never block")
	}

	// Normal window 22:00-06:00 wraps midnight.
	if !IsWithinQuietHours(at(23, 30), 1320, 360) {
		t.Fatal("23:30 must be inside 22:00-06:00 quiet hours")
	}
	if !IsWithinQuietHours(at(2, 0), 1320, 360) {
		t.Fatal("02:00 must be inside overnight quiet hours")
	}
	if IsWithinQuietHours(at(12, 0), 1320, 360) {
		t.Fatal("12:00 must be outside overnight quiet hours")
	}

	// Normal window 21:00-07:00 boundary checks.
	if !IsWithinQuietHours(at(21, 0), 1260, 420) {
		t.Fatal("window start is inclusive")
	}
	if IsWithinQuietHours(at(7, 0), 1260, 420) {
		t.Fatal("window end is exclusive")
	}

	// Daytime window 09:00-18:00.
	if !IsWithinQuietHours(at(9, 0), 540, 1080) {
		t.Fatal("09:00 must be inside daytime quiet hours")
	}
	if IsWithinQuietHours(at(8, 59), 540, 1080) {
		t.Fatal("08:59 must be outside daytime quiet hours")
	}
}

func TestRedactAccessDetails(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "access placeholder hidden",
			in:   "Your stay is confirmed. Door code: {access_code}.",
			want: "Your stay is confirmed. Door code: {access_details_hidden}.",
		},
		{
			name: "pin placeholder hidden",
			in:   "Entry PIN is {access_pin} and wifi is {wifi_password}.",
			want: "Entry PIN is {access_details_hidden} and wifi is {access_details_hidden}.",
		},
		{
			name: "non access placeholder preserved",
			in:   "Hello {guest_name}, booking ref {booking_ref}.",
			want: "Hello {guest_name}, booking ref {booking_ref}.",
		},
		{
			name: "literal code value masked",
			in:   "Smart lock PIN: 482913",
			want: "Smart lock PIN: [hidden]",
		},
		{
			name: "code value after 'is' masked",
			in:   "Your entry code is 482913",
			want: "Your entry code is [hidden]",
		},
		{
			name: "plain text untouched",
			in:   "Your cleaning visit is booked for Friday at 10am.",
			want: "Your cleaning visit is booked for Friday at 10am.",
		},
		{
			name: "password value masked",
			in:   "wifi password = HomeNet2026",
			want: "wifi password = [hidden]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactAccessDetails(tc.in)
			if got != tc.want {
				t.Fatalf("RedactAccessDetails(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
			for _, secret := range []string{"482913", "HomeNet2026"} {
				if strings.Contains(got, secret) {
					t.Fatalf("preview leaked access value %q: %q", secret, got)
				}
			}
		})
	}
}
