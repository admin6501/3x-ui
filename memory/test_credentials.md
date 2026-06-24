# Test Credentials — 3x-ui (v3.4.0 + RBAC)

## Default panel admin (fresh install / empty DB)
- Username: `admin`
- Password: `admin`
- Role: `super_admin` (set automatically on first run / backfilled on upgrade)

## How to run locally for testing
- Build frontend (Node 22 at /opt/node22/bin): `cd /app/frontend && npm run build`
- Build binary (Go 1.26 at /usr/local/go/bin): `cd /app && go build -o /tmp/x-ui-rbac .`
- Run: `XUI_DB_FOLDER=/tmp/xui /tmp/x-ui-rbac`  → panel http://localhost:2053
- Login flow (curl): GET `/` to get the `<meta name="csrf-token">` + cookie,
  then POST `/login` with header `X-CSRF-TOKEN` and JSON `{username,password}`.
  For subsequent POSTs, GET `/panel/csrf-token` to refresh the token.

## RBAC roles
super_admin (full) · manager (inbounds/clients, no panel settings/admins) ·
reseller (only AllowedInbounds-scoped inbounds, clients only) ·
readonly (GET only, all writes 403).
