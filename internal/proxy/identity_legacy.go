package proxy

import "strings"

// parseLegacyPlatformAccountIdentity parses legacy "Platform:Account" identity.
// Legacy behavior is intentionally isolated in this file so the LEGACY_V0 path
// can be removed as a single unit later.
func parseLegacyPlatformAccountIdentity(identity string) (string, string) {
	if idx := strings.IndexByte(identity, ':'); idx >= 0 {
		return identity[:idx], identity[idx+1:]
	}
	return identity, ""
}

// parseLegacyAuthDisabledIdentityCredential parses credentials when
// RESIN_AUTH_VERSION=LEGACY_V0 and RESIN_PROXY_TOKEN is empty.
//
// Accepted legacy-compatible shapes:
//  1. "platform:account"
//  2. "token:platform:account"
//
// This parser intentionally stays independent from V1 parsing code to keep
// legacy behavior explicit and easy to remove in future versions.
func parseLegacyAuthDisabledIdentityCredential(credential string) (platform string, account string) {
	// Direct legacy "platform:account" identity.
	if strings.Count(credential, ":") == 1 {
		return parseLegacyPlatformAccountIdentity(credential)
	}
	// Backward-compatible legacy "token:platform:account" shape.
	if idx := strings.IndexByte(credential, ':'); idx >= 0 {
		return parseLegacyPlatformAccountIdentity(credential[idx+1:])
	}
	return parseLegacyPlatformAccountIdentity(credential)
}
