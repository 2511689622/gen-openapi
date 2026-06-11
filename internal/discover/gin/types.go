package gin

import "gen-openapi/pkg/contract"

// RouteCandidate represents a lightweight route found in a Gin router group.
type RouteCandidate struct {
	Method     string
	Path       string
	Full       string // the full matched call for debugging
	Parameters []contract.Parameter
}
