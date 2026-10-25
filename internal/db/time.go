package db

import "time"

// TimeLayout is the stored form of every timestamp: UTC, whole seconds.
const TimeLayout = "2006-01-02T15:04:05Z"

// FormatTime is t in the stored form of a timestamp.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// ParseTime reads an RFC 3339 instant, such as a stored timestamp, as UTC.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// Flag is b as the 0 or 1 an INTEGER flag column stores.
func Flag(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
