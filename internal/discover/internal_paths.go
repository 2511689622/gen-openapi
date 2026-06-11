package discover

import "strings"

// InternalPathPrefixes is the conservative default list of paths the discover
// step refuses to surface in the canonical api-contract.yaml.
//
// The contract is owner-maintained, so these paths are kept *out* of it by
// default — but DESIGN.md §"维护流程 §3" still wants us to surface them in
// PR comments under a "未纳入候选" / "not exposed" section so a human can
// decide whether they really should be exposed via APIG.
//
// Extend rather than narrow. Anything ambiguous should be on this list.
var InternalPathPrefixes = []string{
	"/debug/",       // /debug/pprof, /debug/vars, ...
	"/metrics",      // Prometheus default
	"/internal/",    // generic internal admin
	"/healthz",      // k8s liveness
	"/readyz",       // k8s readiness
	"/livez",        // k8s liveness alt
	"/-/",           // prometheus/influx style admin
	"/admin/",       // generic admin surface
	"/swagger/",     // generated API docs should not be published through APIG by default
	"/.well-known/", // ACME / OIDC discovery rarely belongs on a product gateway
}

// IsInternalPath reports whether the given path looks like an internal /
// operational endpoint that should not be auto-promoted to the public
// contract.
//
// Matching is case-insensitive. A prefix that ends in "/" matches anything
// underneath it ("/internal/" matches "/internal/reload" but not
// "/internalblog"). A prefix that does NOT end in "/" matches either an
// exact path ("/metrics") or one immediately followed by "/" ("/metrics/v2"),
// so "/metricsboard" stays public.
func IsInternalPath(p string) bool {
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	for _, pref := range InternalPathPrefixes {
		if strings.HasSuffix(pref, "/") {
			if strings.HasPrefix(lower, pref) {
				return true
			}
			continue
		}
		// Non-slash-terminated prefix: exact match OR "prefix/<anything>".
		if lower == pref || strings.HasPrefix(lower, pref+"/") {
			return true
		}
	}
	return false
}
