# 3x-ui — Quota-Exhaustion Disconnect Fix

## Original Problem (translated from user, fa → en)
> "I'll give you a link to a GitHub project. Please review it and, if you can,
> fork it under my GitHub. There is a bug: when a user's data quota runs out,
> they keep being able to use the proxy until I manually restart the Xray core.
> Please fix this consumption issue for me."
>
> Link: https://github.com/MHSanaei/3x-ui
>
> Clarified by user:
> - Use the "Save to GitHub" feature on Emergent to push to the user's fork.
> - Expected behaviour when quota is exhausted: **Option A — the user is
>   disabled immediately and any of their active connections are dropped.**

## Stack
- Language: Go 1.26
- Project: 3x-ui (xray-core based proxy panel)
- Cloned from: https://github.com/MHSanaei/3x-ui

## Root Cause Analysis
1. Every 10s, `web/job/xray_traffic_job.go::Run()` collects traffic from xray
   and calls `inboundService.AddTraffic(...)` which internally invokes
   `disableInvalidClients()`.
2. `disableInvalidClients()` flags exhausted/expired clients with
   `enable=false` in the DB, updates `inbounds.settings`, and calls
   `xrayApi.RemoveUser(tag, email)` over the gRPC handler service.
3. **Critical xray-core limitation**: `RemoveUser` only removes the user from
   the inbound's auth/account list. It does **NOT** kill already-established
   sessions. Existing TCP/TLS sessions belonging to the just-removed user keep
   forwarding traffic until the underlying connection is closed.
4. The only reliable way xray-core supports per-user disconnect today is a
   full process restart (the new process simply won't accept those clients
   because the rebuilt config in `XrayService.GetXrayConfig()` filters them
   out via the `enableMap`).
5. The existing code gated the post-disable restart behind the
   `restartXrayOnClientDisable` setting:
   - If `true` → restart immediately. ✅
   - If `false` → **do nothing**, so leaked sessions of over-quota users kept
     working until the next manual/auto restart. ❌ (the reported bug)

## Fix (single-file change)
File: `web/job/xray_traffic_job.go`

When `clientsDisabled == true`:
- If the setting is `true` (default), force a restart as before.
- If the setting read fails, default to `true` (safe behaviour).
- If the setting is explicitly `false`, **fallback to scheduling** a restart
  via `xrayService.SetToNeedRestart()`. The `@every 30s` cron in
  `web/web.go::startTask()` will then execute `RestartXray(false)` on the next
  tick, which in turn rebuilds the config (now without the disabled clients)
  and drops all active sessions.

Net effect: there is no longer any code path where over-quota / expired
clients can keep proxying through their stale session indefinitely. With
defaults, disconnect is immediate; with the legacy opt-out, disconnect happens
within ≤30s instead of waiting for a manual restart.

## Verification
- `go vet ./web/job/` → clean.
- `go build ./...` → succeeds.
- `go test ./web/job/ ./xray/` → all existing tests pass.
- Manual reasoning: `RestartXray(true)` was already proven (used by the
  original branch); we're just ensuring it is reached on every disable cycle
  one way or another.

## Deployment
- User will use Emergent's "Save to GitHub" to publish this repo as their own
  fork. After deployment, they should rebuild & restart x-ui as usual
  (`./x-ui restart` or `systemctl restart x-ui`) so the new binary is in use.
- The default value of the setting `restartXrayOnClientDisable` remains
  `true`, so first-time installs are unaffected. Existing installs that had
  it set to `false` will now also get an automatic deferred restart instead
  of leaking sessions.

## Files Modified
- `web/job/xray_traffic_job.go` — fixed the `clientsDisabled` branch.
- `install.sh`, `update.sh`, `x-ui.sh` — every download URL now points at the
  user's fork `admin6501/3x-ui` (raw.githubusercontent + api.github.com +
  github.com/releases/download).
- `README.md` + 5 translated READMEs — the top-level install command points
  at the user's fork. Attribution links (badges / wiki / buymeacoffee /
  starchart) left pointing upstream.
- `install_offline.sh` (new) + `offline/x-ui-linux-amd64.tar.gz` (new) +
  `offline/README.md` (new) + `.gitignore` — three-file offline install
  flow for Iranian VPS with no internet. The tarball contains a
  statically-linked x-ui binary (with the quota-exhaust fix), xray-linux-
  amd64, geo .dat files, and systemd units. The installer performs zero
  network calls and skips acme/SSL (can be enabled from the panel later).

## IMPORTANT — Releases must exist on the fork
`install.sh` and `update.sh` download the prebuilt binary from
`https://github.com/admin6501/3x-ui/releases/download/<tag>/x-ui-linux-<arch>.tar.gz`.
GitHub forks do **not** copy releases automatically. So after pushing the
fork, the user must either:

1. Create a release on `admin6501/3x-ui` with the same tag (e.g. `v2.x.y`)
   and upload `x-ui-linux-amd64.tar.gz` / `x-ui-linux-arm64.tar.gz`
   built from this codebase. (Easiest path: enable GitHub Actions, run the
   existing release workflow under `.github/workflows/release.yml`.)
2. OR build locally on the VPS from source:
   ```
   git clone -b main https://github.com/admin6501/3x-ui /usr/local/src/3x-ui
   cd /usr/local/src/3x-ui
   go build -o /usr/local/x-ui/x-ui main.go
   cp x-ui.sh /usr/bin/x-ui && chmod +x /usr/bin/x-ui /usr/local/x-ui/x-ui
   cp x-ui.service.debian /etc/systemd/system/x-ui.service   # or .arch / .rhel
   systemctl daemon-reload && systemctl enable --now x-ui
   ```
   This bypasses the release-tarball download entirely.

## Backlog / Possible Enhancements (not in scope of this fix)
- P2: Add a short cooldown (e.g. don't restart more than once per N seconds)
  if pathological churn is observed in deployments with many simultaneous
  expirations. Current behaviour already limits restarts to once per 10s job
  cycle and disabled rows are not re-picked, so this is mostly defensive.
- P2: Expose a per-inbound “graceful drain” option in the panel so admins can
  choose whether the auto-restart should also affect non-expired users
  (currently it does, because xray-core has no per-user connection-kill API).
- P3: When the user-facing settings page is opened, surface a hint near the
  `restartXrayOnClientDisable` toggle explaining that turning it off will
  delay (not skip) the disconnect of over-quota clients.

## Smart Improvement Suggestion
While we're at it — would you like me to add a small Telegram/webhook
notification when a client is auto-disabled by quota or expiry? It's a tiny
add-on (uses the existing `tgbot` plumbing) and gives admins instant
visibility into who got cut off, which usually translates to faster renewals
and happier (paying) customers.

## Changelog
- 2026-06 — **Re-fork onto upstream v3.4.0 + re-implement the RBAC "admin
  manager" feature in the new architecture**. Upstream MHSanaei/3x-ui jumped
  from the fork base (2.9.4) to **v3.4.0**, which is a full rewrite:
    * Go code moved from `web/` + `database/` to `internal/...` and the module
      path changed `…/v2` → `…/v3`.
    * The server-rendered Vue-in-Go-template UI was replaced by a compiled
      **React 19 + Ant Design 6 + Vite 8 + TypeScript + TanStack Query + zod**
      SPA under `frontend/src/`, embedded into the binary via `internal/web/dist`.
  Because of this, the old `admins.html`-based feature could not be re-applied
  as a patch — it was re-implemented from scratch on the new stack:
    * Backend (Go, `internal/`):
        - `internal/database/model/model.go` — Role constants
          (super_admin/manager/reseller/readonly), `User.Role`,
          `User.AllowedInbounds`, `AdminAuditLog` model.
        - `internal/database/db.go` — AdminAuditLog migration + default-admin
          role + repair-on-startup backfill of empty roles → super_admin.
        - `internal/web/session/session.go` — `HasRole`/`IsSuperAdmin`/`CanWrite`.
        - `internal/web/controller/base.go` — `requireRole`/`requireSuperAdmin`/
          `guardWriteMethods` middlewares.
        - `internal/web/controller/access.go` (new) — reseller inbound scoping
          (`filterInboundsForRole`, `filterInboundOptionsForRole`,
          `scopeInboundParam`, `rejectReseller`, `enforceInboundScope`).
        - `internal/web/controller/admin.go` (new) + `internal/web/service/admin.go`
          (new) — admin CRUD + audit log, mounted at `/panel/api/admin/*`
          (super_admin only).
        - `internal/web/controller/api.go` — mounts admin group, applies
          `guardWriteMethods` (readonly = no writes) on `/panel/api`, gates
          xray-settings to super_admin.
        - `internal/web/controller/setting.go` — gates update/restartPanel/
          apiTokens*/testSmtp/testTgBot to super_admin.
        - `internal/web/controller/inbound.go` — reseller list filtering +
          per-:id scope guards + reject create/import/bulkDel/resetAll.
        - `internal/web/controller/dist.go` — injects `window.X_UI_ROLE` into
          the SPA shell so the frontend can hide role-gated UI.
        - `internal/web/controller/spa.go` — serves the `/panel/admins` route.
    * Frontend (React/TS):
        - `frontend/src/pages/admins/AdminsPage.tsx` (new) — antd Table + modals
          for add/edit/reset-password/delete + audit-log table, TanStack Query.
        - `frontend/src/routes.tsx` — `/admins` route.
        - `frontend/src/layouts/AppSidebar.tsx` — "Admins" menu item, shown
          only when `window.X_UI_ROLE === 'super_admin'`.
        - `frontend/src/env.d.ts` — `X_UI_ROLE` global typing.
        - `internal/web/translation/{en-US,fa-IR}.json` — `menu.admins` +
          `pages.admins.*` strings (other locales fall back to en-US).
  Extra port (per user request, excluding sub-page branding):
    * `internal/web/job/xray_traffic_job.go` — when `restartXrayOnClientDisable`
      is explicitly false, schedule a deferred restart (`SetToNeedRestart`)
      instead of doing nothing, so over-quota/expired clients are disconnected
      on the next cron tick rather than proxying until a manual restart.
    * NOTE: WireGuard subscription-link export is already native in v3.4.0
      (`internal/sub/service.go`), so it was NOT re-ported.
  Distribution (3b): `install.sh`, `update.sh`, `x-ui.sh` download/release URLs
  and the top-level install command in all 7 READMEs now point at
  `admin6501/3x-ui` (branch `main`); badges/wiki/attribution left upstream.
  Verified end-to-end against a running binary on :2053: super_admin lists/
  creates/audits admins (allowedInbounds normalized & deduped), reseller is
  403'd on admin + inbound-create and sees a filtered inbound list, readonly
  can GET but is 403'd on writes; `go vet` clean, frontend typecheck/lint/build
  clean, full Go binary builds (CGO sqlite) and boots.
  PENDING: offline installer + prebuilt offline tarball (`install_offline.sh`
  / `offline/*`) for air-gapped VPS were NOT rebuilt yet (arch-specific,
  ~71 MB bundle incl. xray-core + geo). Needs target-arch confirmation.
- 2026-02 — **Revert: reseller traffic-quota feature removed (per user request)**.
  All code introduced by commit `5986b201` (feat reseller-quota) and
  every follow-up patch on top of it was restored to the pre-feature
  state (parent commit `6f86dc06`). Reverted files:
    * `database/model/model.go` — drops `TrafficQuota` / `TrafficUsed`
      columns from the `User` model
    * `web/service/admin.go` — drops `AccumulateUsage`, `RefundUsage`,
      `CheckResellerQuota`, `ResetTrafficUsage`, `GetUserByID`,
      `RecalculateResellerQuota`, `ResellerRemaining`
    * `web/controller/admin.go` — drops `/resetTrafficUsage/:id` and
      `/recalculateQuota/:id` routes
    * `web/controller/inbound.go` — drops `getMyQuota`, all
      `snapshotRefundFor*` helpers, `applyRefund`, and the
      AccumulateUsage / RefundUsage call sites
    * `web/controller/util.go` — drops `current_user_traffic_*`
      template fields
    * `web/html/admins.html` — drops the Traffic Quota column /
      progress bar, the unit-selector radio, and the
      doResetTrafficUsage button
    * `web/html/inbounds.html` — drops the "Reseller Quota" card
      and the WS quota-refresh listener
    * `web/service/inbound.go` — drops the `CheckResellerQuota` call
      in addClient
    * `web/websocket/hub.go` — drops `MessageTypeAdmins`
    * `web/translation/translate.{en_US,fa_IR}.toml` — drops the
      i18n strings for quota labels
    * `offline/x-ui-linux-amd64.tar.gz` — restored to pre-feature
      binary (MD5 `82bf7e3ab5786b73d8fc23cca53d78a5`)
  Other RBAC features (multi-admin roles, audit log, allowed-inbounds
  scoping, reseller add-inbound restriction) remain intact. My
  follow-up test helpers (`admin_refund_test.go`, `e2e_refund_test.py`)
  were deleted since they referenced the removed methods.
- 2026-02 — **FINAL: Per-event explicit refund (consumed = debt)**.
  Switched back from auto-reconcile to explicit per-event accounting
  per user clarification: "مصرف قبلی رو در نظر بگیرد" — past consumed
  traffic should remain as the reseller's debt, only the *unused*
  portion of a deleted client's allocation returns to the credit.
  Implementation:
    * `computeClientRefund(totalGB, up, down) = max(0, totalGB - up - down)`
    * Three `snapshotRefundFor*` helpers in
      `web/controller/inbound.go` parse the inbound settings JSON with
      the same protocol-specific key lookup the service layer uses
      (id / password / email / auth) so no client is missed.
    * `delInboundClient`, `delInboundClientByEmail`, `delInbound`,
      `delDepletedClients`, and the negative-delta branch of
      `updateInboundClient` all pre-snapshot the refund amount and
      call `applyRefund(ownerId, refund)` after the parent operation
      succeeds. RefundUsage writes a `quota_refund` audit row.
    * `addClient` already calls `AccumulateUsage` → bills the full
      allocation immediately. Net effect:
        - Add 5 GB client → `traffic_used += 5 GB`
        - Client consumes 2 GB
        - Delete client → `traffic_used -= 3 GB (unused refund)`
        - Final state: `traffic_used += 2 GB` (the actual consumed
          bandwidth — operator's bandwidth was real, so reseller pays).
  End-to-end verified with the exact billing scenario above. AMD64
  tarball rebuilt; new binary MD5 `78991507f759d8137a7549fe0eca0d0f`.
- 2026-02 — **Automatic, instant reseller refund on delete (no admin button)**.
  Final design after user pushback: the manual "Recalculate Quota"
  button was removed; instead the controller transparently calls
  `RecalculateResellerQuota(ownerId)` after every successful client /
  inbound delete (and after `updateInboundClient` lowers a client's
  `totalGB`). This means:
    * The reseller deletes a 2 GB client → `traffic_used` immediately
      drops by the FULL 2 GB regardless of how much was consumed.
      Past consumption isn't held against them — it's the operator's
      bandwidth cost, not reseller debt. (User explicitly requested
      this billing model.)
    * Auto self-heals historical drift: any existing reseller whose
      counter was wrong (from earlier panel versions that deleted
      clients without refunding) will be corrected the next time
      any client under them is deleted. No admin intervention needed.
    * Works for every protocol (vless/vmess/trojan/shadowsocks/
      hysteria/hysteria2) because the reconciler reads the canonical
      DB state, not a pre-computed snapshot.
    * Works regardless of which delete endpoint is hit:
      `delInbound`, `delInboundClient`, `delInboundClientByEmail`,
      `delDepletedClients`, and the negative-delta path of
      `updateInboundClient` all call `reconcileOwnerQuota`.
  Audit-log noise reduced: `quota_recalculate` only writes a row when
  `oldUsed != newUsed`. AMD64 tarball rebuilt; new binary MD5
  `3c6ad7b72dda4941e8274ac073a6340b`. End-to-end verified including
  the user's exact drift scenario: 1.9 GB stale value with one 200 MB
  client → reseller adds and deletes a new client → counter auto-
  corrects to 100 MB (the actual unused remainder).
- 2026-02 — **Drift-recovery "Recalculate Quota" feature**. User
  reported their reseller's `traffic_used` showed 1.90 GB while
  actual all-time usage was only 372 MB — clearly drifted (older
  panel versions / pre-fix deletes / interrupted ops). New endpoint
  `POST /panel/admin/recalculateQuota/:id` (super_admin only) walks
  every inbound owned by the reseller, parses each client's `totalGB`
  from the settings JSON, looks up consumed `up + down` from
  `client_traffics`, and recomputes the authoritative `traffic_used`
  as `Σ max(0, totalGB - consumed)`. Returns `{oldUsed, newUsed}` so
  the UI can show a clear before/after toast. Writes a
  `quota_recalculate` audit log entry. Exposed in the admins panel
  as a new "Recalculate" button next to "Reset Quota" — both on
  desktop and the mobile card list. AMD64 tarball rebuilt; new
  binary MD5 `e8ee2ed5bf7b269e8eda4aaa790165c0`. Verified end-to-end
  drift scenario: reseller with `traffic_used=2040109465 (1.9 GB)`
  and only one 200 MB client (172 MB consumed) → recalculate brings
  it to `29360128 (28 MB)` = correct unused remainder.
- 2026-02 — **Hardened reseller-quota flow + audit log**. User
  reported that the refund-on-delete + live-quota-card fixes weren't
  visible on their deployed VPS. Root cause was tentative — likely
  WS connection silently broken (reverse proxy stripping Upgrade
  headers, or container cold-start before listener attached).
  Defensive improvements:
  1. **10-second polling fallback** on `inbounds.html` that always
     calls `refreshMyQuota()` regardless of WS state. Single tiny GET
     to `/panel/api/inbounds/myQuota` (~80 bytes). Guarantees the
     quota card converges within at most 10 s of any backend change.
  2. **Audit log every quota mutation**: `AccumulateUsage` writes a
     `quota_accumulate` row, `RefundUsage` writes a `quota_refund`
     row, with `bytes=...` in details. Now the operator can open the
     panel's audit log and see concrete proof of every billing /
     refund event keyed by reseller, with timestamps.
  3. **Hysteria `Auth` matcher** added to `delInboundClient` refund
     lookup — previously the matcher only covered ID / Password /
     Email, so refunds for Hysteria clients were silently skipped.
  AMD64 tarball rebuilt; new binary MD5 `dafb1b5355a45d01912ddd7a994114e5`.
- 2026-02 — **Inbounds-page "Reseller Quota" card is now live**.
  Reported: the reseller's quota progress card on the inbounds page
  kept showing the stale `0 B / 2 GB` after the reseller added a
  client; the value only refreshed on full page reload. Root cause:
  `resellerQuota` and `resellerUsed` computed properties were reading
  baked-in Go template literals (`{{ .current_user_traffic_quota }}`)
  which are a snapshot at page-load. Fix:
  1. New `GET /panel/api/inbounds/myQuota` returns the freshly-read
     `{role, trafficQuota, trafficUsed}` for the logged-in user.
  2. `web/html/inbounds.html` introduces reactive
     `currentUserTrafficQuota` / `currentUserTrafficUsed` data fields
     seeded from the template, and a `refreshMyQuota()` method that
     polls the new endpoint and updates them. The computed properties
     now read from this reactive state.
  3. `submit()` (the wrapper every mutating client/inbound action goes
     through) calls `refreshMyQuota()` on success — so addClient /
     updateClient / delClient / resetClient* / resetAllTraffics all
     refresh the card automatically.
  4. The existing WebSocket `invalidate{type:admins}` listener (added
     for the admins page) now also calls `refreshMyQuota()` — so a
     super_admin in another tab clicking "Reset Quota" on a reseller
     also refreshes that reseller's quota card live.
  Verified end-to-end in Playwright: card flipped from `0 B / 2.00 GB`
  to `256.00 MB / 2.00 GB` with a partially-filled blue progress bar
  immediately after the reseller added a 256 MB client. AMD64 tarball
  rebuilt.
- 2026-02 — **Admins page auto-refresh on reseller traffic mutations**.
  Reported: when a reseller adds a client to their assigned inbound,
  the admins page (open in a super_admin's tab) didn't reflect the
  freshly-billed quota until a manual reload. Fix: introduce a new
  `websocket.MessageTypeAdmins` and broadcast `invalidate{type:admins}`
  from `AdminService.AccumulateUsage`, `AdminService.RefundUsage`,
  and `AdminService.ResetTrafficUsage` — i.e. every code path that
  mutates `traffic_used`. On the frontend, `admins.html` now subscribes
  to `wsClient.on('invalidate', ...)` after mount, debounces bursts of
  600 ms, and re-calls `loadAdmins() + loadAudit()`. If the WebSocket
  never connects, falls back to a 15 s background poll so the page is
  eventually consistent. End-to-end verified via Playwright: super_admin
  saw `0 / 2 GB` → `512 MB / 2 GB` immediately after a separate
  reseller session POSTed `/panel/api/inbounds/addClient`. AMD64 tarball
  rebuilt.
- 2026-02 — **Reseller quota refund on client/inbound delete**. When
  a client is deleted, the unused portion of its allocated quota
  (`max(0, client.totalGB - (client.up + client.down))`) is now
  returned to the owning reseller's `traffic_used` counter. Deleting
  an entire inbound applies the same per-client refund to every
  client inside, summed. Implemented via new `AdminService.RefundUsage`
  (SQL-level `max(0, traffic_used - bytes)` so older clients whose
  allocations predate the billing system can never push the counter
  negative) and a `computeClientRefund` helper in
  `web/controller/inbound.go`. Hooked into `delInbound`,
  `delInboundClient`, and `delInboundClientByEmail`. Refund is keyed
  off the *inbound owner* (via `Inbound.UserId`), not the actor — so
  a super_admin or manager deleting a reseller's client still refunds
  the reseller correctly. Reset-traffic flows intentionally remain
  unchanged (resets still bill the reseller — see comments in
  `resetClientTraffic`). Unit-tested in
  `web/service/admin_refund_test.go` and end-to-end verified against a
  running panel for the three scenarios: partial-use delete (3 GiB
  refund of 5 GiB allocation), full-inbound delete (2.5 GiB summed),
  and floor-at-zero edge case (10 GiB refund attempt → traffic_used
  clamped to 0). AMD64 tarball rebuilt at
  `/app/offline/x-ui-linux-amd64.tar.gz`.
- 2026-02 — **Admins page mobile UX overhaul**. On desktop the admins
  page stays unchanged (antd `<a-table>` with horizontal scroll). On
  viewports ≤ 768px (the same breakpoint `MediaQueryMixin.isMobile`
  uses everywhere else) both the admins table AND the audit-log table
  are replaced with stacked card lists rendered by a `v-if="isMobile"`
  branch in `web/html/admins.html`. Each admin card surfaces every
  column the desktop row shows (role tag, allowed-inbounds tags,
  quota progress bar for capped resellers, consumed traffic) plus a
  2-column grid of full-width action buttons (Edit / Reset Password
  / Reset Traffic for capped resellers / Delete). The create-edit
  modal and reset-password modal now use `:width="isMobile ? '95%' : 520"`
  so phone keyboards don't clip the form. The Add Admin button in
  the page header becomes block-width on phones via the existing
  `.admins-header-row` CSS. AMD64 tarball rebuilt at
  `/app/offline/x-ui-linux-amd64.tar.gz` (v2.9.16).
- 2026-02 — Offline installer now auto-installs base prerequisites.
  `install_offline.sh` previously assumed `cron`/`curl`/`tar`/`tzdata`/
  `socat`/`ca-certificates`/`openssl` were already present and bailed if
  any tool was missing. It now mirrors `install.sh::install_base` (same
  package names per-distro: cron/cronie/dcron, etc.) so a fresh minimal
  VPS gets the same baseline as the online installer. Power users on
  air-gapped hosts can set `OFFLINE_SKIP_DEPS=1` to opt out.
- 2026-02 — Proxy Chain feature reverted at user request. Removed the
  dedicated "Proxy Chain" form item from outbound editor and the
  `proxyChainTag` getter/setter on the Outbound model. Kept the JSON cleanup
  improvements (`SockoptStreamSettings.toJson` only emits non-default fields,
  `StreamSettings.toJson`/`Outbound.toJson` drop empty sockopt blocks) since
  those are correctness wins independent of the UX revert.
- 2026-02 — Telegram bot proxy can now reference an Xray outbound.
  Settings → "Proxy and Server" gains a dropdown of saved Xray outbound
  tags (filtered to skip blackhole/api). Picking one stores
  `tgBotProxy = "outbound:<tag>"`; on save the panel auto-injects a
  loopback SOCKS5 inbound (tag `x-ui-tgbot-socks`, 127.0.0.1:62792) and a
  matching routing rule into the Xray template, so the bot's traffic
  exits via the chosen outbound. Cleared/changed selections are
  applied idempotently, and `XraySettingService.SaveXraySetting`
  re-applies the injection on every save so an admin can't accidentally
  wipe it from the xray template editor. New endpoint
  `GET /panel/setting/getXrayOutboundTags` (super-admin only) feeds the
  dropdown. Unit-tested in `web/service/tgbot_outbound_test.go`.
  Version bumped to 2.9.11; AMD64 tarball rebuilt at
  `/app/offline/x-ui-linux-amd64.tar.gz`.
