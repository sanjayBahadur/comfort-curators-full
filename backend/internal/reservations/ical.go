package reservations

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// icalEvent is the result of structurally parsing a single VEVENT block.
// Timezone resolution happens later in normalize.go so that the property
// timezone (used for all-day boundaries) can be supplied by the caller.
type icalEvent struct {
	UID         string
	Summary     string
	Description string
	Status      string
	Sequence    int
	AllDay      bool
	StartRaw    string
	EndRaw      string
	Duration    string
	StartTZID   string
	EndTZID     string
	Recurring   bool
	Raw         string
}

var (
	errMissingUID      = errors.New("VEVENT missing UID")
	errMissingDTStart  = errors.New("VEVENT missing DTSTART")
	errInvalidDateTime = errors.New("invalid iCalendar date-time")
	errInvalidDuration = errors.New("invalid iCalendar duration")
	errEndBeforeStart  = errors.New("VEVENT end is before start")
	errRecurringEvent  = errors.New("recurring VEVENT not expanded in MVP")
)

// ParseICal parses the iCalendar (RFC 5545) text and returns the contained
// VEVENT blocks. Recurring events and malformed events are skipped and
// reported through skipErrors; they never abort a poll.
func ParseICal(content string) (events []icalEvent, skipErrors []error, err error) {
	if strings.TrimSpace(content) == "" {
		return nil, nil, ErrEmptyFeedContent
	}

	lines, err := unfoldICalLines(content)
	if err != nil {
		return nil, nil, err
	}

	components := extractComponents(lines)

	var parsed []icalEvent
	for _, block := range components {
		props, err := parseProperties(block)
		if err != nil {
			return nil, nil, err
		}
		ev, perr := buildIcalEvent(props, block)
		if perr != nil {
			skipErrors = append(skipErrors, perr)
			continue
		}
		parsed = append(parsed, ev)
	}
	return parsed, skipErrors, nil
}

// unfoldICalLines joins folded content lines (RFC 5545 section 3.1).
func unfoldICalLines(content string) ([]string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")

	var lines []string
	for _, raw := range rawLines {
		raw = strings.TrimRight(raw, "\r")
		if len(raw) == 0 {
			continue
		}
		if raw[0] == ' ' || raw[0] == '\t' {
			if len(lines) == 0 {
				return nil, fmt.Errorf("%w: continuation line without a start line", ErrInvalidCalendarContent)
			}
			lines[len(lines)-1] += raw[1:]
			continue
		}
		lines = append(lines, raw)
	}
	return lines, nil
}

// extractComponents returns the complete text of every VEVENT block in the
// calendar. A small BEGIN/END stack keeps nested components balanced so each
// VEVENT is captured independently even inside the wrapping VCALENDAR.
func extractComponents(lines []string) []string {
	type component struct {
		name  string
		start int
	}

	var blocks []string
	var stack []component
	for i, line := range lines {
		name, _, value, ok := splitProperty(line)
		if !ok {
			continue
		}
		switch name {
		case "BEGIN":
			stack = append(stack, component{name: strings.ToUpper(value), start: i})
		case "END":
			if len(stack) > 0 && stack[len(stack)-1].name == strings.ToUpper(value) {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.name == "VEVENT" {
					blocks = append(blocks, strings.Join(lines[top.start:i+1], "\n"))
				}
			}
		}
	}
	return blocks
}

// splitProperty splits a content line into name, params and value.
func splitProperty(line string) (name, params, value string, ok bool) {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return "", "", "", false
	}
	head := line[:colon]
	value = line[colon+1:]
	if semi := strings.Index(head, ";"); semi >= 0 {
		name = head[:semi]
		params = head[semi+1:]
	} else {
		name = head
	}
	name = strings.ToUpper(name)
	if name == "" {
		return "", "", "", false
	}
	return name, params, value, true
}

// splitParams parses "A=B;C=D" into a map. Param values may be quoted.
func splitParams(raw string) map[string]string {
	params := make(map[string]string)
	if raw == "" {
		return params
	}
	for _, pair := range strings.Split(raw, ";") {
		eq := strings.Index(pair, "=")
		if eq < 0 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(pair[:eq]))
		val := strings.Trim(strings.TrimSpace(pair[eq+1:]), `"`)
		params[key] = val
	}
	return params
}

func parseProperties(block string) (map[string]string, error) {
	props := make(map[string]string)
	lines, err := unfoldICalLines(block)
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		name, params, value, ok := splitProperty(line)
		if !ok {
			continue
		}
		p := splitParams(params)
		key := name
		if p["VALUE"] == "DATE" {
			key += ";VALUE=DATE"
		}
		props[key] = unescapeICalText(value)
		if tzid := p["TZID"]; tzid != "" {
			props["TZID;"+name] = unescapeICalText(tzid)
		}
	}
	return props, nil
}

var escReplacements = map[string]string{
	"\\\\": "\\",
	"\\;":  ";",
	"\\,":  ",",
	"\\n":  "\n",
	"\\N":  "\n",
}

func unescapeICalText(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); {
		if i+1 < len(value) && value[i] == '\\' {
			pair := value[i : i+2]
			if repl, ok := escReplacements[pair]; ok {
				b.WriteString(repl)
				i += 2
				continue
			}
		}
		b.WriteByte(value[i])
		i++
	}
	return b.String()
}

func buildIcalEvent(props map[string]string, raw string) (icalEvent, error) {
	ev := icalEvent{Raw: raw}

	uid := props["UID"]
	if uid == "" {
		return ev, errMissingUID
	}
	ev.UID = uid
	ev.Summary = props["SUMMARY"]
	ev.Description = props["DESCRIPTION"]

	switch strings.ToUpper(props["STATUS"]) {
	case "TENTATIVE":
		ev.Status = EventStatusTentative
	case "CANCELLED":
		ev.Status = EventStatusCancelled
	default:
		ev.Status = EventStatusConfirmed
	}

	if seq, err := strconv.Atoi(strings.TrimSpace(props["SEQUENCE"])); err == nil {
		ev.Sequence = seq
	}

	if props["RRULE"] != "" || props["RECURRENCE-ID"] != "" {
		ev.Recurring = true
		return ev, fmt.Errorf("%w: %s", errRecurringEvent, uid)
	}

	startRaw := props["DTSTART"]
	startValue := props["DTSTART;VALUE=DATE"]
	if startValue != "" {
		startRaw = startValue
		ev.AllDay = true
	}
	if startRaw == "" {
		return ev, fmt.Errorf("%w: %s", errMissingDTStart, uid)
	}
	ev.StartRaw = startRaw
	ev.StartTZID = props["TZID;DTSTART"]

	ev.EndRaw = props["DTEND"]
	if ev.EndRaw == "" {
		ev.EndRaw = props["DTEND;VALUE=DATE"]
	}
	if ev.EndRaw == "" {
		ev.Duration = props["DURATION"]
	}
	ev.EndTZID = props["TZID;DTEND"]

	return ev, nil
}

func parseICalDate(raw string) (time.Time, error) {
	t, err := time.Parse("20060102", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s", errInvalidDateTime, raw)
	}
	return t, nil
}

func parseICalDateTime(raw string) (time.Time, error) {
	raw = strings.ToUpper(raw)
	raw = strings.TrimSuffix(raw, "Z")
	t, err := time.Parse("20060102T150405", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s", errInvalidDateTime, raw)
	}
	return t, nil
}

func parseDuration(raw string) (time.Duration, error) {
	d := strings.ToUpper(strings.TrimSpace(raw))
	if !strings.HasPrefix(d, "P") {
		return 0, fmt.Errorf("%w: %s", errInvalidDuration, raw)
	}
	d = d[1:]
	if d == "" {
		return 0, fmt.Errorf("%w: %s", errInvalidDuration, raw)
	}

	var days, hours, minutes, seconds int64
	// Split date part (before T) and time part (after T).
	timePart := ""
	if idx := strings.Index(d, "T"); idx >= 0 {
		timePart = d[idx+1:]
		d = d[:idx]
	}

	// Date part handles W, D, M, Y.
	dateTokens := parseDurationTokens(d)
	for _, tok := range dateTokens {
		switch tok.unit {
		case 'W':
			days += tok.num * 7
		case 'D':
			days += tok.num
		case 'M', 'Y':
			return 0, fmt.Errorf("%w: %s", errInvalidDuration, raw)
		}
	}

	timeTokens := parseDurationTokens(timePart)
	for _, tok := range timeTokens {
		switch tok.unit {
		case 'H':
			hours += tok.num
		case 'M':
			minutes += tok.num
		case 'S':
			seconds += tok.num
		}
	}

	total := time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
	return total, nil
}

type durationToken struct {
	num  int64
	unit byte
}

func parseDurationTokens(s string) []durationToken {
	var tokens []durationToken
	var num strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			num.WriteByte(c)
			continue
		}
		n, _ := strconv.ParseInt(num.String(), 10, 64)
		tokens = append(tokens, durationToken{num: n, unit: c})
		num.Reset()
	}
	return tokens
}
