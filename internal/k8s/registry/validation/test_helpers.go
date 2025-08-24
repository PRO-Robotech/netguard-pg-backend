package validation

import "strings"

// Helper function to check if a string contains a substring (case-sensitive)
func containsString(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	return strings.Contains(s, substr)
}
