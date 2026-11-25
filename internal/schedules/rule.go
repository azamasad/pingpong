package schedules

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rule is a daily or weekly schedule rule in Mane local time.
type Rule struct {
	Every   string `json:"every"` // day | week
	At      string `json:"at"`    // HH:MM
	Weekday string `json:"weekday,omitempty"`
	hour    int
	minute  int
	weekday time.Weekday
}

var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
	"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// ParseRuleJSON decodes and validates rule JSON.
func ParseRuleJSON(raw string) (Rule, error) {
	var r Rule
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return Rule{}, fmt.Errorf("bad rule")
	}
	return ValidateRule(r)
}

// ValidateRule checks a decoded rule.
func ValidateRule(r Rule) (Rule, error) {
	switch r.Every {
	case "day", "week":
	default:
		return Rule{}, fmt.Errorf("bad rule")
	}
	if r.Every == "week" {
		wd, ok := weekdayNames[strings.ToLower(r.Weekday)]
		if !ok {
			return Rule{}, fmt.Errorf("bad rule")
		}
		r.weekday = wd
	}
	parts := strings.Split(r.At, ":")
	if len(parts) != 2 {
		return Rule{}, fmt.Errorf("bad rule")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return Rule{}, fmt.Errorf("bad rule")
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return Rule{}, fmt.Errorf("bad rule")
	}
	r.hour, r.minute = h, m
	return r, nil
}

// WallTime returns local wall-clock time on y-m-d for this rule.
func (r Rule) WallTime(y int, m time.Month, d int, loc *time.Location) time.Time {
	return time.Date(y, m, d, r.hour, r.minute, 0, 0, loc)
}

// NextDueAfter returns the first due instant strictly after ref in loc.
func (r Rule) NextDueAfter(ref time.Time, loc *time.Location) time.Time {
	ref = ref.In(loc)
	if r.Every == "day" {
		t := r.WallTime(ref.Year(), ref.Month(), ref.Day(), loc)
		if t.After(ref) {
			return t
		}
		d := ref.AddDate(0, 0, 1)
		return r.WallTime(d.Year(), d.Month(), d.Day(), loc)
	}
	// weekly: next occurrence of weekday after ref
	for i := 0; i < 8; i++ {
		d := ref.AddDate(0, 0, i)
		if d.Weekday() != r.weekday {
			continue
		}
		t := r.WallTime(d.Year(), d.Month(), d.Day(), loc)
		if t.After(ref) {
			return t
		}
	}
	d := ref.AddDate(0, 0, 7)
	return r.WallTime(d.Year(), d.Month(), d.Day(), loc)
}

// LatestDueAtOrBefore returns the latest scheduled instant on or before t in loc.
func (r Rule) LatestDueAtOrBefore(t time.Time, loc *time.Location) (time.Time, bool) {
	t = t.In(loc)
	if r.Every == "day" {
		cand := r.WallTime(t.Year(), t.Month(), t.Day(), loc)
		if !cand.After(t) {
			return cand, true
		}
		d := t.AddDate(0, 0, -1)
		return r.WallTime(d.Year(), d.Month(), d.Day(), loc), true
	}
	for i := 0; i < 8; i++ {
		d := t.AddDate(0, 0, -i)
		if d.Weekday() != r.weekday {
			continue
		}
		cand := r.WallTime(d.Year(), d.Month(), d.Day(), loc)
		if !cand.After(t) {
			return cand, true
		}
	}
	return time.Time{}, false
}

// DueBetween returns scheduled instants in (after, upTo] in loc, oldest first.
func (r Rule) DueBetween(after, upTo time.Time, loc *time.Location) []time.Time {
	after = after.In(loc)
	upTo = upTo.In(loc)
	if !upTo.After(after) {
		return nil
	}
	var out []time.Time
	cur := r.NextDueAfter(after, loc)
	for !cur.After(upTo) {
		out = append(out, cur)
		cur = r.NextDueAfter(cur, loc)
	}
	return out
}
