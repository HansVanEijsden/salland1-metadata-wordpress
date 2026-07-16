package utils

import (
	"time"
)

// FormatHosts formats a list of host names into a string with proper conjunctions
// Examples:
// [] -> ""
// ["John"] -> "John"
// ["John", "Jane"] -> "John en Jane"
// ["John", "Jane", "Bob"] -> "John, Jane en Bob"
// ["John", "Jane", "Bob", "Alice"] -> "John, Jane, Bob, & Team"
func FormatHosts(hosts []string) string {
	if len(hosts) == 0 {
		return ""
	}

	if len(hosts) == 1 {
		return hosts[0]
	}

	if len(hosts) == 2 {
		return hosts[0] + " en " + hosts[1]
	}

	if len(hosts) == 3 {
		return hosts[0] + ", " + hosts[1] + " en " + hosts[2]
	}

	// More than 3 hosts
	return hosts[0] + ", " + hosts[1] + ", " + hosts[2] + ", & Team"
}

// FormatTime converts an ISO-8601 time string to "H:i" format
func FormatTime(isoTime string) string {
	if isoTime == "" {
		return ""
	}

	// Try different time formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, isoTime); err == nil {
			return t.Format("15:04")
		}
	}

	// If parsing fails, return original string as fallback
	return isoTime
}
