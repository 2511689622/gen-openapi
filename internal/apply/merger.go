// Package apply merges routes from a detected candidate contract into a
// canonical ApiContract without overwriting owner-curated fields.
//
// # Overview
//
// Use Merge() to produce a new canonical contract from the old canonical +
// the detected candidate. Merge uses diff.Compare under the hood to classify
// every route, then applies a conservative merge policy:
//
//   - Added routes:         copied from detected (with path params auto-declared,
//     backendPath filled, and referenced schemas copied)
//   - Changed routes:       canonical auth / summary / description / backendPath
//     are preserved; new parameters from detected are
//     appended; request body is only added if canonical
//     has none
//   - Removed routes:       kept by default (pass Prune=true to remove)
//   - NotExposed (internal): skipped by default (pass IncludeInternal=true to promote)
//
// The merged contract is validated via [config.ValidateContract] before being
// returned, so callers can safely write it to disk.
package apply

import (
	"fmt"
	"regexp"
	"strings"

	"gen-openapi/internal/config"
	"gen-openapi/internal/diff"
	"gen-openapi/pkg/contract"

	"gopkg.in/yaml.v3"
)

// pathParamRe matches path parameter placeholders like {id} or {name}. The
// captured group is the parameter name.
var pathParamRe = regexp.MustCompile(`\{([A-Za-z0-9._-]+)\}`)

// Options controls the merge behaviour.
type Options struct {
	// Prune removes routes from canonical that no longer exist in detected.
	// By default removed routes are kept.
	Prune bool

	// IncludeInternal promotes internal paths (/debug, /metrics, /internal/*)
	// from detected into canonical alongside normal added routes.
	// By default they are skipped.
	IncludeInternal bool
}

// Report describes what the merge operation did.
type Report struct {
	Added       []diff.Change // routes copied from detected into canonical
	Merged      []diff.Change // routes that existed in both and were refined
	Pruned      []diff.Change // routes removed from canonical (only with Prune)
	Skipped     []diff.Change // routes intentionally not acted on
	AddedSchema []string      // schema names copied from detected
}

func (r *Report) routeCount() int {
	return len(r.Added) + len(r.Merged) + len(r.Pruned) + len(r.Skipped)
}

// Merge merges routes from detected into canonical according to opts.
//
// canonical may be nil (treated as an empty contract — every public detected
// route becomes an addition). detected must not be nil.
//
// The returned *contract.ApiContract is a deep copy — it shares no memory
// with the inputs. The caller owns it and may write it to disk.
func Merge(canonical, detected *contract.ApiContract, opts Options) (*contract.ApiContract, *Report, error) {
	if detected == nil {
		return nil, nil, fmt.Errorf("detected contract must not be nil")
	}

	// Phase 1: diff
	dr := diff.Compare(canonical, detected)

	// Phase 2: deep-copy canonical (or start empty)
	merged := deepCopyContract(canonical)
	if merged == nil {
		merged = &contract.ApiContract{
			APIVersion: "infra.example.com/v1",
			Kind:       "ApiContract",
			Metadata: contract.Metadata{
				Name:    "unknown",
				Title:   "unknown",
				Version: "1.0.0",
			},
			Spec: contract.Spec{
				BasePath: "/api",
				Schemas:  map[string]*contract.Schema{},
			},
		}
	}
	// Ensure schemas map is initialised
	if merged.Spec.Schemas == nil {
		merged.Spec.Schemas = map[string]*contract.Schema{}
	}

	// Safe-guard: rebuild route list from scratch
	// We'll keep all unchanged canonical routes first, then process each category.
	var kept []contract.Route
	touched := map[diff.RouteKey]bool{}

	rep := &Report{}

	// -- Removed (Phase 3a) --
	for _, c := range dr.Removed {
		touched[c.Key] = true
		if opts.Prune {
			rep.Pruned = append(rep.Pruned, c)
		} else {
			// Keep the canonical route in the output
			if c.Contract != nil {
				kept = append(kept, *c.Contract)
			}
			rep.Skipped = append(rep.Skipped, c)
		}
	}

	// -- Changed (Phase 3b) --
	for _, c := range dr.Changed {
		touched[c.Key] = true
		mergedRoute := mergeChangedRoute(c, &merged.Spec)
		kept = append(kept, mergedRoute)
		rep.Merged = append(rep.Merged, c)
	}

	// -- NotExposed (Phase 3c) --
	for _, c := range dr.NotExposed {
		touched[c.Key] = true
		if opts.IncludeInternal {
			route := prepareAddedRoute(c.Detected, merged.Spec.BasePath)
			kept = append(kept, route)
			copySchemaRefs(route, detected, merged, rep)
			rep.Added = append(rep.Added, c)
		} else {
			rep.Skipped = append(rep.Skipped, c)
		}
	}

	// -- Added (Phase 3d) --
	for _, c := range dr.Added {
		touched[c.Key] = true
		route := prepareAddedRoute(c.Detected, merged.Spec.BasePath)
		kept = append(kept, route)
		copySchemaRefs(route, detected, merged, rep)
		rep.Added = append(rep.Added, c)
	}

	// Keep untouched canonical routes
	for i := range merged.Spec.Routes {
		r := merged.Spec.Routes[i]
		k := diff.RouteKey{Method: strings.ToUpper(r.Method), Path: r.Path}
		if !touched[k] {
			kept = append(kept, r)
		}
	}

	merged.Spec.Routes = kept

	// Phase 5: validate
	if err := config.ValidateContract(merged); err != nil {
		return nil, rep, fmt.Errorf("merged contract is invalid: %w", err)
	}

	return merged, rep, nil
}

// ---------------------------------------------------------------------------
// merge helpers
// ---------------------------------------------------------------------------

// mergeChangedRoute applies conservative merge for a route present in both
// canonical and detected.
func mergeChangedRoute(c diff.Change, spec *contract.Spec) contract.Route {
	// Start from canonical
	r := *c.Contract

	// Never overwrite owner-curated fields
	// auth, summary, description, backendPath stay as-is

	// Append new parameters from detected that don't exist in canonical
	existing := map[string]bool{}
	for _, p := range r.Parameters {
		key := strings.ToLower(p.In) + ":" + p.Name
		existing[key] = true
	}
	if c.Detected != nil {
		for _, p := range c.Detected.Parameters {
			key := strings.ToLower(p.In) + ":" + p.Name
			if !existing[key] {
				r.Parameters = append(r.Parameters, p)
				existing[key] = true
			}
		}
	}

	// Add requestBody from detected only if canonical has none
	if r.RequestBody == nil && c.Detected != nil && c.Detected.RequestBody != nil {
		rb := *c.Detected.RequestBody
		r.RequestBody = &rb
	}

	// Ensure path params are declared
	ensurePathParams(&r)

	return r
}

// prepareAddedRoute copies a detected route and normalises it for the
// canonical contract.
func prepareAddedRoute(detected *contract.Route, basePath string) contract.Route {
	r := *detected

	// Ensure path params are declared
	ensurePathParams(&r)

	// Fill backendPath if missing
	if r.BackendPath == "" && basePath != "" {
		r.BackendPath = basePath + r.Path
	}

	return r
}

// ensurePathParams scans the route path for {param} placeholders and adds
// parameter declarations for any that are missing.
func ensurePathParams(r *contract.Route) {
	declared := map[string]bool{}
	for _, p := range r.Parameters {
		if strings.EqualFold(p.In, "path") {
			declared[p.Name] = true
		}
	}
	for _, match := range pathParamRe.FindAllStringSubmatch(r.Path, -1) {
		name := match[1]
		if !declared[name] {
			r.Parameters = append(r.Parameters, contract.Parameter{
				Name:     name,
				In:       "path",
				Type:     "string",
				Required: true,
			})
			declared[name] = true
		}
	}
}

// copySchemaRefs copies any schemas referenced by the route's request body
// from src into dst.
func copySchemaRefs(r contract.Route, src, dst *contract.ApiContract, rep *Report) {
	if r.RequestBody == nil || r.RequestBody.Schema == "" {
		return
	}
	copySchema(r.RequestBody.Schema, src, dst, rep)
}

// copySchema copies a single schema by name from src into dst if dst does
// not already have it. It also recursively scans the schema for $ref entries
// and copies the referenced schemas too.
func copySchema(name string, src, dst *contract.ApiContract, rep *Report) {
	if name == "" {
		return
	}
	if _, exists := dst.Spec.Schemas[name]; exists {
		return // canonical already has it, don't overwrite
	}
	s, ok := src.Spec.Schemas[name]
	if !ok {
		return // not in detected either
	}
	// Mark as existing first to prevent infinite recursion on cyclic refs
	dst.Spec.Schemas[name] = nil // placeholder
	rep.AddedSchema = append(rep.AddedSchema, name)

	cloned := deepCopySchema(s)
	if cloned == nil {
		// remove placeholder on failure
		delete(dst.Spec.Schemas, name)
		return
	}
	dst.Spec.Schemas[name] = cloned

	// Recursively resolve $ref entries
	visitRefs(cloned, func(ref string) {
		copySchema(ref, src, dst, rep)
	})
}

// visitRefs walks a schema tree and calls fn for each $ref value found.
// It extracts the schema name from $ref values like "#/components/schemas/X"
// or bare "X".
func visitRefs(s *contract.Schema, fn func(ref string)) {
	if s == nil {
		return
	}
	if s.Ref != "" {
		// Strip "#/components/schemas/" prefix if present
		ref := s.Ref
		if idx := strings.LastIndex(ref, "/"); idx >= 0 {
			ref = ref[idx+1:]
		}
		fn(ref)
	}
	for _, prop := range s.Properties {
		visitRefs(prop, fn)
	}
	if s.Items != nil {
		visitRefs(s.Items, fn)
	}
}

// ---------------------------------------------------------------------------
// deep copy helpers
// ---------------------------------------------------------------------------

// deepCopyContract returns a deep copy of c via yaml round-trip. If c is nil
// or unmarshal fails, returns nil.
func deepCopyContract(c *contract.ApiContract) *contract.ApiContract {
	if c == nil {
		return nil
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return nil
	}
	var out contract.ApiContract
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil
	}
	return &out
}

// deepCopySchema returns a deep copy of s via yaml round-trip. If s is nil
// or unmarshal fails, returns nil.
func deepCopySchema(s *contract.Schema) *contract.Schema {
	if s == nil {
		return nil
	}
	b, err := yaml.Marshal(s)
	if err != nil {
		return nil
	}
	var out contract.Schema
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil
	}
	return &out
}
