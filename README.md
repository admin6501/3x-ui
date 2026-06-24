[English](/README.md) | [فارسی](/README.fa_IR.md) | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

# 3X-UI — RBAC Fork (admin6501)

A customized fork of [MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui) — an advanced, open-source web panel for managing [Xray-core](https://github.com/XTLS/Xray-core) servers — extended with a built-in **multi-admin RBAC system**, **reseller scoping**, and a **fully offline installer** for air-gapped or restricted servers.

> [!IMPORTANT]
> This project is intended for personal use only. Please do not use it for illegal purposes or in a production environment.

## What this fork adds

- **Role-Based Access Control (RBAC)** — manage multiple panel administrators from the **Admins** page, each with one of four roles:
  - `super_admin` — full access to everything (the first/default admin).
  - `manager` — manage inbounds & clients; **no** panel settings, Xray template, or admin management.
  - `reseller` — scoped to assigned inbounds only; manages their own clients and **views** their inbounds read-only (cannot add / edit / enable-disable / delete inbounds).
  - `readonly` — can view everything but cannot perform any write action.
- **Audit log** — every admin action (create / update / delete admin, password reset, …) is recorded with actor, target, and timestamp.
- **Offline install bundle** — install on a server with no internet using a self-contained tarball (panel binary + Xray-core + geo data). See [`offline/`](offline/).
- **Fork-aware updater** — the panel's "check for update" reads releases from this fork (`admin6501/3x-ui`).
- **PostgreSQL-safe migration** — SQLite → PostgreSQL migration copies all RBAC data (admins, roles, allowed inbounds, audit logs).

## Core features (from 3X-UI)

- **Multi-protocol inbounds** — VLESS, VMess, Trojan, Shadowsocks, WireGuard, Hysteria2, HTTP, SOCKS (Mixed), Dokodemo-door / Tunnel, and TUN.
- **Modern transports & security** — TCP (Raw), mKCP, WebSocket, gRPC, HTTPUpgrade, and XHTTP, secured with TLS, XTLS, and REALITY.
- **Per-client management** — traffic quotas, expiry dates, IP limits, live online status, share links, QR codes, and subscriptions.
- **Multi-node support**, outbound & routing (WARP, custom rules, load balancers, proxy chaining).
- **Built-in subscription server** with [custom page templates](docs/custom-subscription-templates.md).
- **Telegram bot**, **RESTful API** with in-panel Swagger, **SQLite or PostgreSQL**, **13 UI languages**, dark/light themes, and **Fail2ban** IP-limit enforcement.

## Quick Start (online)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/admin6501/3x-ui/main/install.sh)
```

A random username, password, and web base path are generated during install. Run `x-ui` afterwards to open the management menu (start/stop, reset credentials, manage SSL, etc.).

## Offline install (no internet)

For servers with no internet access — or where the GitHub download is blocked:

1. Copy `offline/x-ui-linux-amd64.tar.gz` and `install_offline.sh` to the server (any folder).
2. Run:

```bash
chmod +x install_offline.sh
sudo ./install_offline.sh                 # auto-detects the .tar.gz in the current dir
# or pass it explicitly:
sudo ./install_offline.sh /root/x-ui-linux-amd64.tar.gz
```

It makes **zero network calls** for the bundle (binary + Xray + geo all come from the tarball), prints the generated credentials **and the API token**, and on upgrade **preserves your existing `/etc/x-ui/x-ui.db`** while running migrations (including the RBAC tables). See [`offline/README.md`](offline/README.md) for details.

> The prebuilt offline bundle is **amd64 / x86_64 only**. For other architectures, build a bundle for that arch from source.

## Supported platforms

**Operating systems:** Ubuntu, Debian, Armbian, Fedora, CentOS, RHEL, AlmaLinux, Rocky Linux, Oracle Linux, Amazon Linux, Arch, Manjaro, openSUSE, Alpine, and Windows.

**Architectures:** `amd64` · `arm64` (aarch64) · `armv7` · `armv6` · `386` · `s390x`.

## Database

- **SQLite** (default) — a single file at `/etc/x-ui/x-ui.db`. Zero setup, ideal for small/medium deployments.
- **PostgreSQL** — recommended for high client counts or multi-node setups.

Migrate an existing SQLite install to PostgreSQL (all RBAC data included):

```bash
x-ui migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
# then set XUI_DB_TYPE and XUI_DB_DSN in /etc/default/x-ui and restart:
systemctl restart x-ui
```

The source SQLite file is left untouched; remove it once you have verified the new backend.

## Environment Variables

| Variable | Description | Default |
| --- | --- | --- |
| `XUI_DB_TYPE` | Database backend: `sqlite` or `postgres` | `sqlite` |
| `XUI_DB_DSN` | PostgreSQL connection string (when `XUI_DB_TYPE=postgres`) | — |
| `XUI_DB_FOLDER` | Directory for the SQLite database file | `/etc/x-ui` |
| `XUI_INIT_WEB_BASE_PATH` | The initial URI path for the web panel | `/` |
| `XUI_ENABLE_FAIL2BAN` | Enable Fail2ban-based IP-limit enforcement | `true` |
| `XUI_LOG_LEVEL` | Log verbosity (`debug`, `info`, `warning`, `error`) | `info` |

## Supported Languages

The panel UI is available in 13 languages:

English · فارسی · العربية · 中文（简体） · 中文（繁體） · Español · Русский · Українська · Türkçe · Tiếng Việt · 日本語 · Bahasa Indonesia · Português (Brasil)

## Documentation

- [Capturing the real client IP](docs/real-client-ip.md) (behind Cloudflare / L4 relays).
- [Custom subscription templates](docs/custom-subscription-templates.md).

## Credits & License

This project is a fork of **[MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui)** and builds on [Xray-core](https://github.com/XTLS/Xray-core) and the original X-UI by [alireza0](https://github.com/alireza0/). Geo routing rules: [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) & [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat).

Licensed under **GPL-3.0**, the same as the upstream project.
