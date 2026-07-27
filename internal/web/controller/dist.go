package controller

import (
	"bytes"
	"embed"
	"encoding/json"
	htmlpkg "html"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

var distFS embed.FS

func SetDistFS(fs embed.FS) {
	distFS = fs
}

var distPageBuildTime = time.Now()

// ServeOpenAPISpec returns the generated OpenAPI 3.0 description of the
// panel API. Postman / Insomnia / openapi-generator consume this URL
// directly; the in-panel Swagger UI page also fetches it. The spec is
// produced at frontend build time by scripts/build-openapi.mjs and
// embedded into the binary via the dist FS.
func ServeOpenAPISpec(c *gin.Context) {
	body, err := distFS.ReadFile("dist/openapi.json")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "msg": "openapi.json not found"})
		return
	}

	// The embedded spec ships with `servers: [{url: "/"}]`. When the panel runs
	// under a non-root web base path, Swagger UI "Try it out" and external
	// generators must target that prefix, so rewrite the single server entry to
	// the runtime base path before serving.
	if basePath := c.GetString("base_path"); basePath != "" && basePath != "/" {
		if rebuilt, err := withServerBasePath(body, basePath); err != nil {
			logger.Warning("openapi.json: could not inject base path:", err)
		} else {
			body = rebuilt
		}
	}

	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// withServerBasePath rewrites the spec's `servers` entry so requests target the
// panel's configured web base path. Only the top-level `servers` field is
// replaced; every other field is preserved verbatim via json.RawMessage.
func withServerBasePath(spec []byte, basePath string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, err
	}
	servers, err := json.Marshal([]map[string]string{{
		"url":         strings.TrimSuffix(basePath, "/"),
		"description": "Current panel",
	}})
	if err != nil {
		return nil, err
	}
	doc["servers"] = servers
	return json.Marshal(doc)
}

func serveDistPage(c *gin.Context, name string) {
	body, err := distFS.ReadFile("dist/" + name)
	if err != nil {
		c.String(http.StatusInternalServerError, "missing embedded page: %s", name)
		return
	}

	basePath := c.GetString("base_path")
	if basePath == "" {
		basePath = "/"
	}

	if basePath != "/" {
		body = bytes.ReplaceAll(body, []byte(`src="/assets/`), []byte(`src="`+basePath+`assets/`))
		body = bytes.ReplaceAll(body, []byte(`href="/assets/`), []byte(`href="`+basePath+`assets/`))
	}

	jsEscape := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"<", `<`,
		">", `>`,
		"&", `&`,
	)
	escapedBase := jsEscape.Replace(basePath)
	csrfToken, err := session.EnsureCSRFToken(c)
	if err != nil {
		logger.Warning("Unable to mint CSRF token for", name+":", err)
		csrfToken = ""
	}
	csrfMeta := []byte(`<meta name="csrf-token" content="` + htmlpkg.EscapeString(csrfToken) + `">`)
	basePathMeta := []byte(`<meta name="base-path" content="` + htmlpkg.EscapeString(basePath) + `">`)

	nonceAttr := ""
	if nonce := c.GetString("csp_nonce"); nonce != "" {
		nonceAttr = ` nonce="` + htmlpkg.EscapeString(nonce) + `"`
	}
	script := `<script` + nonceAttr + `>window.X_UI_BASE_PATH="` + escapedBase + `"`
	if name != "login.html" {
		escapedVer := jsEscape.Replace(config.GetVersion())
		script += `;window.X_UI_CUR_VER="` + escapedVer + `"`
		script += `;window.X_UI_DB_TYPE="` + config.GetDBKind() + `"`
		// Expose the logged-in user's RBAC role so the SPA can hide menu
		// items / actions the role can't use. Empty role (pre-RBAC session)
		// is treated as super_admin to match the backend's HasRole rule.
		role := "super_admin"
		if u := session.GetLoginUser(c); u != nil && u.Role != "" {
			role = u.Role
		}
		script += `;window.X_UI_ROLE="` + jsEscape.Replace(role) + `"`
		// Custom roles are named by the admin who created them, so the SPA gets
		// the display name alongside the raw role key.
		script += `;window.X_UI_ROLE_NAME="` + jsEscape.Replace(service.RoleDisplayName(role)) + `"`
		// The permission set drives which menu entries and actions the SPA
		// offers. It is a convenience for the UI only — every one of these is
		// enforced again on the server.
		perms := session.PermissionsOf(&model.User{Role: role})
		granted := make([]string, 0, len(model.AllPermissions))
		for _, p := range model.AllPermissions {
			if perms[p] {
				granted = append(granted, `"`+jsEscape.Replace(p)+`"`)
			}
		}
		script += `;window.X_UI_PERMS=[` + strings.Join(granted, ",") + `]`
	}
	script += `;</script>`
	inject := []byte(script)
	inject = append(inject, csrfMeta...)
	inject = append(inject, basePathMeta...)
	inject = append(inject, []byte(`</head>`)...)
	out := bytes.Replace(body, []byte("</head>"), inject, 1)

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Last-Modified", distPageBuildTime.UTC().Format(http.TimeFormat))
	c.Data(http.StatusOK, "text/html; charset=utf-8", out)
}
