// Package gin implements lightweight scanners that discover route registrations
// in a Go / Gin codebase. The AST scanner resolves simple Group() prefix chains;
// the regex scanner remains available as a low-dependency fallback.
package gin

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gen-openapi/pkg/contract"
)

// AnalyzeFiles uses Go AST to discover common Gin routes across a map of file
// contents (filename → source bytes). It intentionally does not resolve
// handlers, request bodies, schemas, responses, or auth. Those belong in future
// optional analyzers; this scanner only finds method + path candidates.
func AnalyzeFiles(files map[string][]byte) ([]RouteCandidate, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no Go files to analyze")
	}

	filenames := make([]string, 0, len(files))
	for filename := range files {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)

	var out []RouteCandidate
	var parseErrs []string
	for _, filename := range filenames {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filename, files[filename], 0)
		if err != nil {
			parseErrs = append(parseErrs, err.Error())
			continue
		}
		out = append(out, analyzeFile(f)...)
	}
	if len(out) == 0 && len(parseErrs) > 0 {
		return nil, fmt.Errorf("parse Go files: %s", strings.Join(parseErrs, "; "))
	}
	return out, nil
}

func analyzeFile(file *ast.File) []RouteCandidate {
	var out []RouteCandidate
	ginAliases := ginImportAliases(file)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		groups := map[string]string{}
		routers := ginRouterNames(fn, ginAliases)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				recordAssignedRouter(node, routers, ginAliases)
				recordAssignedGroup(node, groups, routers, ginAliases)
			case *ast.DeclStmt:
				recordDeclaredRouters(node, routers, ginAliases)
				recordDeclaredGroups(node, groups, routers, ginAliases)
			case *ast.CallExpr:
				if route, ok := routeFromCall(node, groups, routers, ginAliases); ok {
					out = append(out, route)
				}
			}
			return true
		})
	}
	return out
}

func recordAssignedGroup(stmt *ast.AssignStmt, groups map[string]string, routers map[string]bool, ginAliases map[string]bool) {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		call, ok := stmt.Rhs[i].(*ast.CallExpr)
		if !ok {
			continue
		}
		if prefix, ok := groupPrefix(call, groups, routers, ginAliases); ok {
			groups[ident.Name] = prefix
			routers[ident.Name] = true
		}
	}
}

func recordDeclaredGroups(stmt *ast.DeclStmt, groups map[string]string, routers map[string]bool, ginAliases map[string]bool) {
	decl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok || decl.Tok != token.VAR {
		return
	}
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			if i >= len(valueSpec.Values) {
				continue
			}
			call, ok := valueSpec.Values[i].(*ast.CallExpr)
			if !ok {
				continue
			}
			if prefix, ok := groupPrefix(call, groups, routers, ginAliases); ok {
				groups[name.Name] = prefix
				routers[name.Name] = true
			}
		}
	}
}

func groupPrefix(call *ast.CallExpr, groups map[string]string, routers map[string]bool, ginAliases map[string]bool) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Group" || len(call.Args) == 0 {
		return "", false
	}
	if !isRouterReceiver(sel.X, groups, routers, ginAliases) {
		return "", false
	}
	p, ok := stringArg(call.Args[0])
	if !ok {
		return "", false
	}
	return joinPaths(receiverPrefix(sel.X, groups, routers, ginAliases), p), true
}

func ginImportAliases(file *ast.File) map[string]bool {
	aliases := map[string]bool{"gin": true}
	for _, imp := range file.Imports {
		pathValue, err := strconv.Unquote(imp.Path.Value)
		if err != nil || pathValue != "github.com/gin-gonic/gin" {
			continue
		}
		alias := "gin"
		if imp.Name != nil && imp.Name.Name != "_" && imp.Name.Name != "." {
			alias = imp.Name.Name
		}
		aliases[alias] = true
	}
	return aliases
}

func ginRouterNames(fn *ast.FuncDecl, ginAliases map[string]bool) map[string]bool {
	routers := map[string]bool{}
	if fn.Type.Params == nil {
		return routers
	}
	for _, field := range fn.Type.Params.List {
		if !isGinRouterType(field.Type, ginAliases) {
			continue
		}
		for _, name := range field.Names {
			routers[name.Name] = true
		}
	}
	return routers
}

func recordAssignedRouter(stmt *ast.AssignStmt, routers map[string]bool, ginAliases map[string]bool) {
	for i, lhs := range stmt.Lhs {
		if i >= len(stmt.Rhs) {
			continue
		}
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		if isGinRouterConstructor(stmt.Rhs[i], ginAliases) {
			routers[ident.Name] = true
		}
	}
}

func recordDeclaredRouters(stmt *ast.DeclStmt, routers map[string]bool, ginAliases map[string]bool) {
	decl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok || decl.Tok != token.VAR {
		return
	}
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			if valueSpec.Type != nil && isGinRouterType(valueSpec.Type, ginAliases) {
				routers[name.Name] = true
				continue
			}
			if i < len(valueSpec.Values) && isGinRouterConstructor(valueSpec.Values[i], ginAliases) {
				routers[name.Name] = true
			}
		}
	}
}

func isGinRouterConstructor(expr ast.Expr, ginAliases map[string]bool) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || !ginAliases[pkg.Name] {
		return false
	}
	return sel.Sel.Name == "New" || sel.Sel.Name == "Default"
}

func isGinRouterType(expr ast.Expr, ginAliases map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return isGinRouterType(e.X, ginAliases)
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		if !ok || !ginAliases[pkg.Name] {
			return false
		}
		return e.Sel.Name == "Engine" || e.Sel.Name == "RouterGroup"
	}
	return false
}

func routeFromCall(call *ast.CallExpr, groups map[string]string, routers map[string]bool, ginAliases map[string]bool) (RouteCandidate, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) == 0 {
		return RouteCandidate{}, false
	}
	method := strings.ToUpper(sel.Sel.Name)
	if !isRouteMethod(method) || !isRouterReceiver(sel.X, groups, routers, ginAliases) {
		return RouteCandidate{}, false
	}
	p, ok := stringArg(call.Args[0])
	if !ok {
		return RouteCandidate{}, false
	}
	path := normalizePath(joinPaths(receiverPrefix(sel.X, groups, routers, ginAliases), p))
	return RouteCandidate{
		Method:     method,
		Path:       path,
		Full:       fmt.Sprintf("%s.%s(%q, ...)", exprName(sel.X), method, p),
		Parameters: pathParameters(path),
	}, true
}

func receiverPrefix(expr ast.Expr, groups map[string]string, routers map[string]bool, ginAliases map[string]bool) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return groups[e.Name]
	case *ast.CallExpr:
		if p, ok := groupPrefix(e, groups, routers, ginAliases); ok {
			return p
		}
	}
	return ""
}

func isRouterReceiver(expr ast.Expr, groups map[string]string, routers map[string]bool, ginAliases map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return routers[e.Name]
	case *ast.CallExpr:
		if isGinRouterConstructor(e, ginAliases) {
			return true
		}
		_, ok := groupPrefix(e, groups, routers, ginAliases)
		return ok
	}
	return false
}

func exprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprName(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			return exprName(sel.X) + "." + sel.Sel.Name + "()"
		}
	}
	return "expr"
}

func isRouteMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH":
		return true
	default:
		return false
	}
}

func stringArg(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

func joinPaths(prefix, p string) string {
	if prefix == "" {
		if p == "" {
			return "/"
		}
		return ensureLeadingSlash(p)
	}
	if p == "" || p == "/" {
		return ensureLeadingSlash(strings.TrimSuffix(prefix, "/"))
	}
	return ensureLeadingSlash(strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(p, "/"))
}

func ensureLeadingSlash(p string) string {
	if p == "" {
		return "/"
	}
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

func pathParameters(path string) []contract.Parameter {
	parts := strings.Split(path, "/")
	params := make([]contract.Parameter, 0)
	seen := map[string]bool{}
	for _, part := range parts {
		if len(part) < 3 || part[0] != '{' || part[len(part)-1] != '}' {
			continue
		}
		name := part[1 : len(part)-1]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		params = append(params, contract.Parameter{
			Name:     name,
			In:       "path",
			Type:     "string",
			Required: true,
		})
	}
	return params
}

// ExtractRoutes scans a Go source file for Gin method-handler registrations
// and returns the routes it found. It does not resolve group prefixes —
// AnalyzeFiles should be preferred when source filenames are available.
//
// Matched patterns (case-insensitive method):
//
//	<any-var>.<METHOD>("/path", ...)
func ExtractRoutes(content []byte) []RouteCandidate {
	text := removeComments(string(content))
	pattern := regexp.MustCompile(`(\w+)\.(GET|POST|PUT|DELETE|PATCH)\s*\(\s*"([^"]+)"`)
	matches := pattern.FindAllStringSubmatch(text, -1)

	var out []RouteCandidate
	for _, m := range matches {
		path := normalizePath(m[3])
		out = append(out, RouteCandidate{
			Method:     m[2],
			Path:       path,
			Full:       m[0],
			Parameters: pathParameters(path),
		})
	}
	return out
}

func removeComments(text string) string {
	reBlock := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	text = reBlock.ReplaceAllString(text, "")
	reLine := regexp.MustCompile(`//[^\n]*`)
	text = reLine.ReplaceAllString(text, "")
	return text
}

// normalizePath converts Gin-style ":param" segments to OpenAPI "{param}".
func normalizePath(p string) string {
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if strings.HasPrefix(seg, ":") {
			parts[i] = "{" + seg[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}
