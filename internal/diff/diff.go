// Package diff compares an owner-maintained ApiContract against a candidate
// contract produced by the discover/import adapters and classifies every
// route into one of:
//
//	added       — present only in the candidate (route the source has but the
//	              contract does not)
//	removed     — present only in the contract (route the contract has but the
//	              source no longer does)
//	changed     — present in both, but parameters / requestBody / auth differ
//	notExposed  — present only in the candidate AND classified as an internal
//	              path by [discover.IsInternalPath]; surfaced separately so a
//	              human reviewer explicitly decides whether to expose it
//
// The output is a [Report] value plus a Markdown renderer; rendering and
// classification are kept separate so the renderer is easy to test and so
// alternative renderers (JSON, GitHub PR comment) can be added later.
package diff

import (
	"fmt"
	"sort"
	"strings"

	"gen-openapi/internal/discover"
	"gen-openapi/pkg/contract"
)

// RouteKey uniquely identifies a route by HTTP method + path.
type RouteKey struct {
	Method string
	Path   string
}

func (k RouteKey) String() string { return k.Method + " " + k.Path }

func key(r contract.Route) RouteKey {
	return RouteKey{Method: strings.ToUpper(r.Method), Path: r.Path}
}

// Change describes a single route-level difference between contract and
// candidate.
type Change struct {
	Key      RouteKey
	Reasons  []string // human-readable bullet points for the changed kind
	Contract *contract.Route
	Detected *contract.Route
}

// Report is the structured diff result. Slices are sorted by RouteKey so
// rendering is deterministic.
type Report struct {
	Added      []Change // in candidate, not in contract, public path
	Removed    []Change // in contract, not in candidate
	Changed    []Change // in both, but operation shape differs
	NotExposed []Change // in candidate only AND internal path
}

// IsEmpty returns true when contract and candidate are equivalent for diff
// purposes. CI can use this to decide whether to fail or open an issue.
func (r Report) IsEmpty() bool {
	return len(r.Added) == 0 && len(r.Removed) == 0 &&
		len(r.Changed) == 0 && len(r.NotExposed) == 0
}

// Compare classifies every route in canonical and candidate.
//
// canonical may be nil (treated as empty); candidate must not be nil.
func Compare(canonical, candidate *contract.ApiContract) Report {
	contractIdx := map[RouteKey]*contract.Route{}
	if canonical != nil {
		for i := range canonical.Spec.Routes {
			r := &canonical.Spec.Routes[i]
			contractIdx[key(*r)] = r
		}
	}

	detectedIdx := map[RouteKey]*contract.Route{}
	for i := range candidate.Spec.Routes {
		r := &candidate.Spec.Routes[i]
		detectedIdx[key(*r)] = r
	}

	var rep Report

	// removed: in contract, not in candidate
	for k, r := range contractIdx {
		if _, ok := detectedIdx[k]; !ok {
			rep.Removed = append(rep.Removed, Change{Key: k, Contract: r})
		}
	}

	// added vs not-exposed
	for k, r := range detectedIdx {
		if _, ok := contractIdx[k]; ok {
			continue
		}
		ch := Change{Key: k, Detected: r}
		if discover.IsInternalPath(k.Path) {
			rep.NotExposed = append(rep.NotExposed, ch)
		} else {
			rep.Added = append(rep.Added, ch)
		}
	}

	// changed: in both, with semantic differences
	for k, contractRoute := range contractIdx {
		detectedRoute, ok := detectedIdx[k]
		if !ok {
			continue
		}
		reasons := routeReasons(contractRoute, detectedRoute)
		if len(reasons) == 0 {
			continue
		}
		rep.Changed = append(rep.Changed, Change{
			Key:      k,
			Reasons:  reasons,
			Contract: contractRoute,
			Detected: detectedRoute,
		})
	}

	sortByKey(rep.Added)
	sortByKey(rep.Removed)
	sortByKey(rep.Changed)
	sortByKey(rep.NotExposed)
	return rep
}

func sortByKey(xs []Change) {
	sort.Slice(xs, func(i, j int) bool {
		if xs[i].Key.Path != xs[j].Key.Path {
			return xs[i].Key.Path < xs[j].Key.Path
		}
		return xs[i].Key.Method < xs[j].Key.Method
	})
}

// routeReasons returns the list of differences between two same-key routes,
// or nil if they are equivalent for our purposes. We deliberately only
// surface differences that owners care about (parameters, requestBody, auth)
// — summary / description drift is too noisy to fail PRs on.
func routeReasons(a, b *contract.Route) []string {
	var out []string

	if a.Auth != b.Auth && !(a.Auth == "" && b.Auth == "none") && !(a.Auth == "none" && b.Auth == "") {
		out = append(out, fmt.Sprintf("auth: %q → %q", a.Auth, b.Auth))
	}

	addedQ, removedQ := paramDelta(a.Parameters, b.Parameters, "query")
	for _, p := range addedQ {
		out = append(out, fmt.Sprintf("added query parameter %s", p))
	}
	for _, p := range removedQ {
		out = append(out, fmt.Sprintf("removed query parameter %s", p))
	}

	addedH, removedH := paramDelta(a.Parameters, b.Parameters, "header")
	for _, p := range addedH {
		out = append(out, fmt.Sprintf("added header %s", p))
	}
	for _, p := range removedH {
		out = append(out, fmt.Sprintf("removed header %s", p))
	}

	addedP, removedP := paramDelta(a.Parameters, b.Parameters, "path")
	for _, p := range addedP {
		out = append(out, fmt.Sprintf("added path parameter %s", p))
	}
	for _, p := range removedP {
		out = append(out, fmt.Sprintf("removed path parameter %s", p))
	}

	bodyA := bodySchema(a)
	bodyB := bodySchema(b)
	if bodyA != bodyB {
		out = append(out, fmt.Sprintf("requestBody schema: %s → %s",
			emptyDash(bodyA), emptyDash(bodyB)))
	}

	return out
}

func bodySchema(r *contract.Route) string {
	if r.RequestBody == nil {
		return ""
	}
	return r.RequestBody.Schema
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// paramDelta returns (added, removed) parameter names for the given `in`
// location (a → b).
func paramDelta(a, b []contract.Parameter, in string) (added, removed []string) {
	aSet := map[string]bool{}
	for _, p := range a {
		if strings.EqualFold(p.In, in) {
			aSet[p.Name] = true
		}
	}
	bSet := map[string]bool{}
	for _, p := range b {
		if strings.EqualFold(p.In, in) {
			bSet[p.Name] = true
		}
	}
	for name := range bSet {
		if !aSet[name] {
			added = append(added, name)
		}
	}
	for name := range aSet {
		if !bSet[name] {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return
}

// Markdown renders the report in the format DESIGN.md §"PR 文案示例" describes.
// It is safe to feed the output directly into a GitHub issue body or PR
// comment.
func Markdown(r Report) string {
	var b strings.Builder
	b.WriteString("# API contract drift\n\n")

	if r.IsEmpty() {
		b.WriteString("_No drift detected._\n")
		return b.String()
	}

	if len(r.Added) > 0 {
		b.WriteString("## 新增候选接口\n\n")
		for _, c := range r.Added {
			fmt.Fprintf(&b, "- `+ %s %s`\n", c.Key.Method, c.Key.Path)
		}
		b.WriteString("\n")
	}

	if len(r.Removed) > 0 {
		b.WriteString("## 移除候选接口\n\n")
		for _, c := range r.Removed {
			fmt.Fprintf(&b, "- `- %s %s`\n", c.Key.Method, c.Key.Path)
		}
		b.WriteString("\n")
	}

	if len(r.Changed) > 0 {
		b.WriteString("## 参数变化\n\n")
		for _, c := range r.Changed {
			fmt.Fprintf(&b, "- `~ %s %s`\n", c.Key.Method, c.Key.Path)
			for _, why := range c.Reasons {
				fmt.Fprintf(&b, "  - %s\n", why)
			}
		}
		b.WriteString("\n")
	}

	if len(r.NotExposed) > 0 {
		b.WriteString("## 未纳入候选（请人工确认是否暴露）\n\n")
		for _, c := range r.NotExposed {
			fmt.Fprintf(&b, "- `* %s %s`\n", c.Key.Method, c.Key.Path)
		}
		b.WriteString("\n")
	}

	return b.String()
}
