package platform

import (
	"fmt"
	"strings"
)

const (
	ServiceFilterOpenAI     = "openai"
	ServiceFilterAnthropic  = "anthropic"
	ServiceFilterUnsupported = "unsupported"
)

// NormalizeServiceFilter trims and lowercases one service filter token.
func NormalizeServiceFilter(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// NormalizeServiceFilters normalizes service filter tokens in order.
func NormalizeServiceFilters(filters []string) []string {
	out := make([]string, 0, len(filters))
	for _, filter := range filters {
		out = append(out, NormalizeServiceFilter(filter))
	}
	return out
}

func isValidServiceFilter(filter string) bool {
	switch filter {
	case ServiceFilterOpenAI, ServiceFilterAnthropic, ServiceFilterUnsupported:
		return true
	default:
		return false
	}
}

// ValidateServiceFilters validates platform service filter values.
func ValidateServiceFilters(filters []string) error {
	for i, filter := range filters {
		normalized := NormalizeServiceFilter(filter)
		if !isValidServiceFilter(normalized) {
			return fmt.Errorf(
				"service_filters[%d]: must be %s, %s, or %s",
				i,
				ServiceFilterOpenAI,
				ServiceFilterAnthropic,
				ServiceFilterUnsupported,
			)
		}
	}
	return nil
}

// MatchServiceFilter checks whether one node capability set matches one service filter token.
func MatchServiceFilter(openai, anthropic bool, filter string) bool {
	switch NormalizeServiceFilter(filter) {
	case ServiceFilterOpenAI:
		return openai
	case ServiceFilterAnthropic:
		return anthropic
	case ServiceFilterUnsupported:
		return !openai && !anthropic
	default:
		return false
	}
}

// MatchServiceFilters applies OR semantics over configured service filters.
// Empty filters mean no service constraint.
func MatchServiceFilters(openai, anthropic bool, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if MatchServiceFilter(openai, anthropic, filter) {
			return true
		}
	}
	return false
}
