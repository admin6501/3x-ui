# Test Credentials

## 3x-ui panel (running on http://localhost:2053, no basePath)

| Username | Password    | Role        | Notes                                                                          |
|----------|-------------|-------------|--------------------------------------------------------------------------------|
| admin    | admin123    | super_admin | Default super-admin                                                            |
| rcapped1 | reseller123 | reseller    | Capped reseller: traffic_quota=2 GiB, allowed_inbounds = inbound id 1 (`test-ib-1`) |

## Auth flow notes (important for testing agent)

- This panel uses **CSRF tokens**. POST requests get 403 Forbidden without one.
- After GET `/`, parse `<meta name="csrf-token" content="...">` from the HTML and send it as the `X-CSRF-Token` request header on every POST.
- Login: `POST /login` with form fields `username` and `password` (NOT JSON body).
- After login, the CSRF token rotates on the next GET — re-parse the meta tag from the next page (`/panel/inbounds` or `/panel/admins`).

## Endpoints used by tests

- `GET  /panel/admin/list` — list admins (super_admin only)
- `POST /panel/admin/resetTrafficUsage/:id` — reset reseller's `traffic_used` to 0
- `GET  /panel/api/inbounds/list` — list inbounds (filtered to allowed_inbounds for reseller)
- `GET  /panel/api/inbounds/myQuota` — **NEW** — returns `{role, trafficQuota, trafficUsed}` for current user
- `POST /panel/api/inbounds/addClient` — add a client (JSON body)
- `POST /panel/api/inbounds/:id/delClientByEmail/:email` — delete client by email
- `POST /panel/api/inbounds/del/:id` — delete an entire inbound

## Test data (auto-seeded by setup script)

- Inbound id 1, tag `test-ib-1`, owned by reseller id 2 (rcapped1).
- One seed client `seed@e2e` (unlimited).
- Reseller quota: 2 GiB, used: 0.
