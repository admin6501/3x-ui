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
- 2026-02 — Proxy Chain UX surfaced (later removed). Added a dedicated
  top-level "Proxy Chain" dropdown in `web/html/form/outbound.html` plus a
  `proxyChainTag` getter/setter on the `Outbound` model that wrote to
  `streamSettings.sockopt.dialerProxy`.
- 2026-02 — Proxy Chain bug fix (later removed). Fixed silent breakage of
  outbound routing when a chain was enabled. Root cause:
  `SockoptStreamSettings.toJson()` always emitted defaults like
  `addressPortStrategy: "none"`, which older Xray-core builds rejected.
  Rewrote sockopt `toJson` to emit only non-default fields, and updated
  `StreamSettings.toJson` / `Outbound.toJson` to drop the sockopt block
  entirely when empty. Version bumped to 2.9.10.
- 2026-05 — Proxy Chain UX removed per user request. Dropped the
  top-level "Proxy Chain" dropdown from `web/html/form/outbound.html`
  and the `proxyChainTag` getter/setter from
  `web/assets/js/model/outbound.js`. The native Xray field
  `streamSettings.sockopt.dialerProxy` is still available through the
  Sockopts panel (`form/stream/stream_sockopt.html`) for advanced users
  who edit JSON directly. The earlier sockopt `toJson` cleanup is kept
  because it's a general improvement unrelated to the Proxy Chain UI.
