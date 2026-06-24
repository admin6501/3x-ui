# Test Results — 3x-ui re-fork onto upstream v3.4.0 + RBAC "Admin Manager"

## user_problem_statement
Fork is `admin6501/3x-ui` (was based on upstream 2.9.4). Upstream released a new
version (v3.4.0 — a full Go `internal/` + React-SPA rewrite). Re-fork onto
v3.4.0 and re-add the custom features: the multi-admin RBAC "admin manager",
plus the other extras that existed in the fork (e.g. WireGuard sub link) —
EXCLUDING the subscription-page branding. Point install scripts at the fork.

## NOTE for future agents (project type)
This is a **Go (1.26.4) + React 19/Vite 8 SPA** project, NOT the platform's
default React/FastAPI/Mongo app. The supervisor services do NOT run it.
To build & run:
  - Go: `/usr/local/go/bin` (go1.26.4). Frontend: Node 22 at `/opt/node22/bin`.
  - Build frontend first (`cd frontend && npm run build` → emits
    `internal/web/dist`, which the Go binary embeds), then `go build -o x-ui .`.
  - Run: `XUI_DB_FOLDER=/tmp/xui ./x-ui` → panel on :2053 (sub on :2096).
  - Default admin: admin / admin (super_admin).
The platform's deep_testing_backend_v2 / frontend agents target the
REACT_APP_BACKEND_URL app and will NOT work against this Go binary on :2053.

## backend (Go RBAC) — validated by main agent via curl against running binary
backend:
  - task: "Admin CRUD + audit log (/panel/api/admin/*, super_admin only)"
    implemented: true
    working: true
    file: "internal/web/controller/admin.go, internal/web/service/admin.go"
    comment: "super_admin: list returns role/allowedInbounds (password redacted); create reseller normalizes/dedupes allowedInbounds '3,1,3,7'->'1,3,7'; audit log records create_admin. All 200 OK."
  - task: "RBAC role gating (super_admin / manager / reseller / readonly)"
    implemented: true
    working: true
    file: "internal/web/controller/base.go, api.go, setting.go, internal/web/session/session.go"
    comment: "reseller -> 403 on /admin/list and inbounds/add; readonly -> GET 200 but POST setting/update 403. super_admin full access."
  - task: "Reseller inbound scoping"
    implemented: true
    working: true
    file: "internal/web/controller/access.go, inbound.go"
    comment: "reseller inbounds/list filtered to AllowedInbounds (empty set => []); per-:id scope guards; create/import/bulkDel/resetAll rejected."
  - task: "DB migration + role injection to SPA"
    implemented: true
    working: true
    file: "internal/database/model/model.go, db.go, internal/web/controller/dist.go"
    comment: "AdminAuditLog migrated; default admin = super_admin; window.X_UI_ROLE injected correctly (super_admin/reseller verified in /panel/ shell)."
  - task: "Quota-exhaustion disconnect fallback"
    implemented: true
    working: "NA"
    file: "internal/web/job/xray_traffic_job.go"
    comment: "When restartXrayOnClientDisable=false, now schedules SetToNeedRestart instead of no-op. Compiles; runtime needs a real xray-core to fully exercise (not available in this dev env)."

## frontend (React Admins page) — built, not yet exercised in a browser by a test agent
frontend:
  - task: "Admins management page + sidebar entry (super_admin only)"
    implemented: true
    working: "NA"
    file: "frontend/src/pages/admins/AdminsPage.tsx, routes.tsx, layouts/AppSidebar.tsx"
    comment: "typecheck + eslint + vite build all clean; AdminsPage chunk emitted. Not yet clicked-through in a browser."

## metadata
  build_status: "frontend vite build OK; go build OK (109MB binary, CGO sqlite); go vet clean; binary boots on :2053"

## agent_communication
  - agent: "main"
    message: "Re-forked onto v3.4.0 and re-implemented RBAC end-to-end on the new Go-internal + React-SPA stack. Backend validated via curl (super_admin/manager/reseller/readonly flows). Frontend builds clean. PENDING: offline installer + prebuilt tarball (arch-specific) not rebuilt yet."
