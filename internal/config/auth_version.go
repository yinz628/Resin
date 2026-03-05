package config

import "strings"

// AuthVersion represents proxy auth parsing behavior.
type AuthVersion string

const (
	AuthVersionLegacyV0 AuthVersion = "LEGACY_V0"
	AuthVersionV1       AuthVersion = "V1"

	// AuthMigrationGuidePath is the in-repo migration guide location.
	AuthMigrationGuidePath = "doc/v1.0.0-migration-guide.md"
	// AuthMigrationGuideURL is the public migration guide link for user-facing hints.
	AuthMigrationGuideURL = "https://github.com/Resinat/Resin/blob/master/doc/v1.0.0-migration-guide.md"
)

// NormalizeAuthVersion trims and normalizes auth version values.
// Returns empty when value is not recognized.
func NormalizeAuthVersion(raw string) AuthVersion {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case string(AuthVersionLegacyV0):
		return AuthVersionLegacyV0
	case string(AuthVersionV1):
		return AuthVersionV1
	default:
		return ""
	}
}
