package validation

import (
	"regexp"
)

// isDNS1123Subdomain validates that the value is a valid DNS-1123 subdomain
// This function is shared across all validators
func isDNS1123Subdomain(value string) bool {
	// DNS-1123 subdomain must consist of lower case alphanumeric characters, '-' or '.',
	// and must start and end with an alphanumeric character
	const dns1123SubdomainFmt string = "[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*"
	const dns1123SubdomainMaxLength int = 253

	if len(value) > dns1123SubdomainMaxLength {
		return false
	}
	if len(value) == 0 {
		return false
	}

	matched, _ := regexp.MatchString("^"+dns1123SubdomainFmt+"$", value)
	return matched
}
