// Package tools shared helpers used across all tool implementations.
package tools

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Time helpers
// ---------------------------------------------------------------------------

// timestampf returns now() in RFC3339 format.
func timestampf() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ttlFromDays returns an RFC3339 expiry timestamp N days from now.
// Returns "" when days <= 0 (no expiry).
func ttlFromDays(days float64) string {
	if days <= 0 {
		return ""
	}
	return time.Now().UTC().Add(time.Duration(days*24) * time.Hour).Format(time.RFC3339)
}

// ttlFromHours returns an RFC3339 expiry timestamp N hours from now.
// Returns "" when hours <= 0.
func ttlFromHours(hours float64) string {
	if hours <= 0 {
		return ""
	}
	return time.Now().UTC().Add(time.Duration(hours) * time.Hour).Format(time.RFC3339)
}

// isExpired returns true if the RFC3339 ttl string represents a time in the past.
// An empty string is treated as no expiry (not expired).
func isExpired(ttl string) bool {
	if ttl == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, ttl)
	if err != nil {
		return false
	}
	return time.Now().UTC().After(t)
}

// humanAge returns a human-readable age string for a created/timestamp field.
// Accepts RFC3339 strings; returns "" on parse error.
func humanAge(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(math.Round(d.Hours() / 24))
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// ageDays returns the fractional number of days since ts (RFC3339).
// Returns 0 on parse error.
func ageDays(ts string) float64 {
	if ts == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0
	}
	return time.Since(t).Hours() / 24
}

// ---------------------------------------------------------------------------
// Relative time parsing
// ---------------------------------------------------------------------------

// parseRelativeTime parses an ISO8601 timestamp or a relative string like
// "7d", "2h", "30m", "1w" and returns the corresponding time.
// Relative strings are interpreted as offsets *before* now.
func parseRelativeTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time string")
	}

	// Try ISO8601 first.
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Relative: ends with d, h, m, w.
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	var val float64
	if _, err := fmt.Sscanf(numStr, "%f", &val); err != nil {
		return time.Time{}, fmt.Errorf("cannot parse relative time %q", s)
	}
	var dur time.Duration
	switch unit {
	case 'm':
		dur = time.Duration(val * float64(time.Minute))
	case 'h':
		dur = time.Duration(val * float64(time.Hour))
	case 'd':
		dur = time.Duration(val * 24 * float64(time.Hour))
	case 'w':
		dur = time.Duration(val * 7 * 24 * float64(time.Hour))
	default:
		return time.Time{}, fmt.Errorf("unknown time unit %q in %q", unit, s)
	}
	return time.Now().UTC().Add(-dur), nil
}

// ---------------------------------------------------------------------------
// Payload helpers
// ---------------------------------------------------------------------------

// tagsToIfaces converts a []interface{} of tag values to a []interface{}
// containing only string elements. Safe to store in Qdrant payload.
func tagsToIfaces(raw []interface{}) []interface{} {
	out := make([]interface{}, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ifacesToStrings extracts []string from a []interface{} payload field.
func ifacesToStrings(raw interface{}) []string {
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// stringsToIfaces converts []string → []interface{} for Qdrant payload storage.
func stringsToIfaces(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// payloadFloat returns a float64 from a payload map field. Returns def if
// the field is absent or not numeric.
func payloadFloat(payload map[string]interface{}, key string, def float64) float64 {
	v, ok := payload[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return def
}

// payloadInt returns an int from a payload map field.
func payloadInt(payload map[string]interface{}, key string, def int) int {
	v, ok := payload[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int64:
		return int(x)
	case int:
		return x
	}
	return def
}

// payloadString returns a string from a payload map field.
func payloadString(payload map[string]interface{}, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}
