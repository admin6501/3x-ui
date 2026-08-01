package controller

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type routeDef struct {
	Method string
	Path   string
}

// routePattern matches route registrations like g.GET("/path", handler) or api.GET("/path", handler)
var routePattern = regexp.MustCompile(`\b(g|api)\.(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\("([^"]+)"`)

// docRoutePattern matches { method: 'X', path: 'Y' ... } entries in endpoints.ts.
var docRoutePattern = regexp.MustCompile(`method:\s*'([A-Z]+)'\s*,\s*path:\s*'([^']+)'`)

// spaPagePattern matches the GET registrations that serve the React shell, so
// UI pages are recognised as such no matter how many are added to spa.go.
var spaPagePattern = regexp.MustCompile(`\bg\.GET\("([^"]+)",\s*a\.panelSPA\)`)

// controllerBasePaths maps each controller file to the router group its routes
// are mounted on in APIController.initRouter (web.go for the root ones), so a
// registration like g.POST("/add") can be resolved to its full path. Files that
// register on the engine root map to "". Keep in sync when a controller moves
// to a different group or a new one is added.
var controllerBasePaths = map[string]string{
	"index.go":           "",
	"websocket.go":       "",
	"spa.go":             "/panel",
	"api.go":             "/panel/api",
	"inbound.go":         "/panel/api/inbounds",
	"client.go":          "/panel/api/clients",
	"group.go":           "/panel/api/clients",
	"server.go":          "/panel/api/server",
	"node.go":            "/panel/api/nodes",
	"host.go":            "/panel/api/hosts",
	"setting.go":         "/panel/api/setting",
	"xray_setting.go":    "/panel/api/xray",
	"admin.go":           "/panel/api/admin",
	"plan.go":            "/panel/api/plans",
	"reseller.go":        "/panel/api/reseller",
	"client_activity.go": "/panel/api/clientActivity",
	"role.go":            "/panel/api/roles",
	"shop.go":            "/panel/api/shop",
}

// buildDocSet parses frontend/src/pages/api-docs/endpoints.ts and returns the
// set of documented "METHOD PATH" keys. WS pseudo-routes and subscription
// placeholders (paths starting with /{...}) are skipped because they aren't
// registered on the main Gin engine.
func buildDocSet(t *testing.T) map[string]bool {
	t.Helper()
	controllerDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("failed to get current dir: %v", err)
	}
	endpointsPath := filepath.Join(controllerDir, "..", "..", "..", "frontend", "src", "pages", "api-docs", "endpoints.ts")
	data, err := os.ReadFile(endpointsPath)
	if err != nil {
		t.Fatalf("failed to read endpoints.ts at %s: %v", endpointsPath, err)
	}
	docSet := make(map[string]bool)
	for _, m := range docRoutePattern.FindAllStringSubmatch(string(data), -1) {
		method, path := m[1], m[2]
		if method == "WS" {
			continue
		}
		if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "/{") {
			continue
		}
		docSet[method+" "+path] = true
	}
	if len(docSet) == 0 {
		t.Fatalf("no documented routes parsed from %s — regex or file format may have changed", endpointsPath)
	}
	return docSet
}

func TestAPIRoutesDocumented(t *testing.T) {
	docSet := buildDocSet(t)

	controllerDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("failed to get current dir: %v", err)
	}

	var allRoutes []routeDef
	// Paths served by the SPA shell (UI pages, not API endpoints). Collected
	// from spa.go itself so adding a page there never fails this test.
	spaPages := map[string]bool{"/": true}

	entries, err := os.ReadDir(controllerDir)
	if err != nil {
		t.Fatalf("failed to read controller dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(controllerDir, entry.Name()))
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		src := string(data)

		// Find all route registrations
		matches := routePattern.FindAllStringSubmatch(src, -1)
		if len(matches) == 0 {
			continue
		}

		// The group each file's routes are mounted on (see APIController.initRouter).
		// A controller that registers routes without an entry here would have its
		// paths mis-derived, so an unknown file is a hard failure rather than a
		// silent mismatch reported as "route not documented".
		basePath, known := controllerBasePaths[entry.Name()]
		if !known {
			t.Errorf("%s registers routes but has no base path in controllerBasePaths — add its api.Group() prefix there", entry.Name())
			continue
		}

		for _, m := range spaPagePattern.FindAllStringSubmatch(src, -1) {
			spaPages[basePath+m[1]] = true
		}

		for _, m := range matches {
			method := m[2]
			path := strings.TrimSpace(m[3])
			if basePath == "" {
				allRoutes = append(allRoutes, routeDef{Method: method, Path: path})
			} else {
				fullPath := basePath + path
				allRoutes = append(allRoutes, routeDef{Method: method, Path: fullPath})
			}
		}
	}

	// The WebSocket route /ws is registered in web/web.go (not a controller file)
	allRoutes = append(allRoutes, routeDef{Method: "GET", Path: "/ws"})

	missingFromDocs := 0
	foundInDoc := 0
	sourceSet := make(map[string]bool)

	for _, r := range allRoutes {
		key := r.Method + " " + r.Path
		// Skip SPA page routes (these are UI pages, not API endpoints)
		if spaPages[r.Path] {
			continue
		}
		// Skip /panel/csrf-token (documented under auth as /csrf-token)
		if r.Path == "/panel/csrf-token" {
			continue
		}
		// Skip Chrome DevTools route
		if strings.Contains(r.Path, ".well-known") {
			continue
		}

		sourceSet[key] = true
		if docSet[key] {
			foundInDoc++
		} else {
			missingFromDocs++
			t.Errorf("Route not documented in endpoints.ts: %s %s", r.Method, r.Path)
		}
	}

	t.Logf("Routes found in source: %d, documented: %d, matching: %d, missing: %d",
		len(sourceSet), len(docSet), foundInDoc, missingFromDocs)

	if missingFromDocs > 0 {
		t.Errorf("Found %d undocumented route(s). Update endpoints.ts to match.", missingFromDocs)
	}
}
