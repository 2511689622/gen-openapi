package discover

import "testing"

func TestIsInternalPath(t *testing.T) {
	cases := map[string]bool{
		// internal
		"/debug/pprof":        true,
		"/debug/vars":         true,
		"/metrics":            true,
		"/metrics/v2":         true,
		"/internal/reload":    true,
		"/healthz":            true,
		"/readyz":             true,
		"/livez":              true,
		"/-/admin":            true,
		"/admin/dashboard":    true,
		"/swagger/index.html": true,
		"/.well-known/openid": true,
		"/Debug/PPROF":        true, // case-insensitive

		// public — must NOT trip the blacklist
		"/v1/pkg":        false,
		"/api/orders":    false,
		"/users/{id}":    false,
		"":               false,
		"/internal-blog": false, // does not start with /internal/
		"/metricsboard":  false, // /metrics matches only exact-or-with-trailing-slash
		"/healthzy":      false, // /healthz is exact + /-suffix only
	}
	for path, want := range cases {
		if got := IsInternalPath(path); got != want {
			t.Errorf("IsInternalPath(%q): want %v, got %v", path, want, got)
		}
	}
}
