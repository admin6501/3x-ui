# 3x-ui — Offline Install Bundle (v3.4.0 + RBAC fork)

This folder ships a **fully self-contained** 3x-ui install for servers that have
**no internet access** (or where the GitHub release download is blocked).

## What's in the bundle

`x-ui-linux-amd64.tar.gz` extracts to an `x-ui/` directory containing:

- `x-ui` — the panel binary (**amd64 / x86_64**). The React web UI is embedded
  inside the binary (no separate assets needed).
- `x-ui.sh` — the admin/management CLI.
- `bin/` — `xray-linux-amd64` (Xray-core v26.6.22) and geo data:
  `geoip.dat`, `geosite.dat`, `geoip_IR.dat`, `geosite_IR.dat`,
  `geoip_RU.dat`, `geosite_RU.dat`.
- `x-ui.service.debian` / `x-ui.service.arch` / `x-ui.service.rhel` — systemd
  units; `x-ui.rc` — OpenRC unit (Alpine).

> Arch note: this prebuilt bundle is **amd64 only**. For arm64/armv7 etc., build
> a bundle for that arch (cross-compile `x-ui` + grab the matching Xray/mtg/geo)
> and name it `x-ui-linux-<arch>.tar.gz`.

## How to install on the target server

Copy two (or three) files to the server — any folder, e.g. `/root`:

1. `x-ui-linux-amd64.tar.gz`
2. `install_offline.sh` (in the repo root)
3. `x-ui.sh` *(optional — the installer falls back to the copy inside the tarball)*

Then:

```bash
chmod +x install_offline.sh
sudo ./install_offline.sh                       # auto-detects the .tar.gz in $PWD
# or pass the tarball explicitly:
sudo ./install_offline.sh /root/x-ui-linux-amd64.tar.gz
```

The installer makes **zero network calls for the x-ui bundle** (binary + xray +
geo all come from the tarball). It *does* try to install base OS packages
(curl, tar, cron, socat, openssl, ca-certificates, tzdata) via the system
package manager so a fresh minimal image isn't left half-working. If the target
truly has no package mirror, skip that step:

```bash
sudo OFFLINE_SKIP_DEPS=1 ./install_offline.sh
```

On first install it prints randomly-generated **username / password / port /
webBasePath** and the access URL. On re-install/upgrade it **preserves the
existing `/etc/x-ui/x-ui.db`** (your admins, inbounds and clients are kept) and
runs `x-ui migrate` to apply any new schema (e.g. the RBAC `admin` columns +
`admin_audit_logs` table added by this fork).

## After install

- `x-ui` — open the admin menu.
- `x-ui status` — check the service.
- The first admin is a **super_admin**; manage additional admins
  (manager / reseller / readonly) from the **Admins** page in the panel.
