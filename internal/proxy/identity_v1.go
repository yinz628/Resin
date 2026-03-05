package proxy

import "strings"

// parseV1PlatformAccountIdentity parses V1 identity segment:
// "Platform.Account" (preferred) or "Platform:Account" (compat).
//
// V1 behavior is intentionally isolated from legacy parsing code so V1 can
// evolve independently and LEGACY_V0 can be removed cleanly in the future.
func parseV1PlatformAccountIdentity(identity string) (string, string) {
	dot := strings.IndexByte(identity, '.')
	colon := strings.IndexByte(identity, ':')
	switch {
	case dot < 0 && colon < 0:
		return identity, ""
	case dot < 0:
		return identity[:colon], identity[colon+1:]
	case colon < 0:
		return identity[:dot], identity[dot+1:]
	case dot < colon:
		return identity[:dot], identity[dot+1:]
	default:
		return identity[:colon], identity[colon+1:]
	}
}

// parseForwardCredentialV1 parses V1 forward credential:
// "<Platform><delimiter><Account>:<TOKEN>" where delimiter is '.' or ':'.
// TOKEN is split using the right-most ':'.
func parseForwardCredentialV1(credential string) (token string, platform string, account string) {
	identity := credential
	if idx := strings.LastIndexByte(credential, ':'); idx >= 0 {
		identity = credential[:idx]
		token = credential[idx+1:]
	}
	platform, account = parseV1PlatformAccountIdentity(identity)
	return token, platform, account
}

// parseForwardCredentialV1WhenAuthDisabled parses optional identity when
// RESIN_AUTH_VERSION=V1 and RESIN_PROXY_TOKEN is empty.
//
// NOTE:
//   - This is V1 parser code and must not call legacy parser functions.
//   - For migration compatibility, colon-only credentials still keep
//     legacy-compatible extraction semantics via a local fallback branch.
func parseForwardCredentialV1WhenAuthDisabled(credential string) (platform string, account string) {
	lastColon := strings.LastIndexByte(credential, ':')
	if lastColon >= 0 {
		identity := credential[:lastColon]
		// Dot in identity indicates explicit V1 shape: Platform.Account:TOKEN.
		if strings.IndexByte(identity, '.') >= 0 {
			_, platform, account = parseForwardCredentialV1(credential)
			return platform, account
		}
	}
	// Colon-only shapes are ambiguous under V1 when token is empty.
	// Keep migration-compatible behavior, but keep the implementation local to V1.
	return parseForwardCredentialV1WhenAuthDisabledLegacyCompat(credential)
}

// parseForwardCredentialV1WhenAuthDisabledLegacyCompat mirrors legacy extraction
// semantics for V1's token-empty migration mode.
//
// This logic intentionally duplicates legacy parsing behavior instead of calling
// legacy parser functions, so V1 and LEGACY_V0 stay structurally decoupled.
func parseForwardCredentialV1WhenAuthDisabledLegacyCompat(credential string) (platform string, account string) {
	// Legacy-compatible two-field shape: "platform:account".
	if strings.Count(credential, ":") == 1 {
		if idx := strings.IndexByte(credential, ':'); idx >= 0 {
			return credential[:idx], credential[idx+1:]
		}
		return credential, ""
	}

	// Legacy-compatible three-field shape: "token:platform:account".
	if idx := strings.IndexByte(credential, ':'); idx >= 0 {
		rest := credential[idx+1:]
		if restIdx := strings.IndexByte(rest, ':'); restIdx >= 0 {
			return rest[:restIdx], rest[restIdx+1:]
		}
		return rest, ""
	}

	return credential, ""
}
