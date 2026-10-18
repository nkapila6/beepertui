package ui

import (
	"fmt"
	"strings"
	"time"
)

var nowFunc = time.Now

// parseWhen understands "30m", "2h", "3d", "tomorrow", "tomorrow 14:00", "17:30" and "2026-10-20 09:00".
func parseWhen(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return time.Time{}, fmt.Errorf("when? try 30m, 2h, 1d, tomorrow, 17:30")
	}
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			return now.AddDate(0, 0, n), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(d), nil
	}
	day := now
	if rest, ok := strings.CutPrefix(s, "tomorrow"); ok {
		day = now.AddDate(0, 0, 1)
		s = strings.TrimSpace(rest)
		if s == "" {
			s = "09:00"
		}
	}
	if t, err := time.ParseInLocation("15:04", s, now.Location()); err == nil {
		at := time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		if !at.After(now) {
			at = at.AddDate(0, 0, 1)
		}
		return at, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("can't parse %q. Try 30m, 2h, 1d, tomorrow, 17:30, 2026-10-20 09:00", s)
}
