# 3x-ui — Offline Install Bundle (admin6501 fork)

This directory contains everything you need to install 3x-ui on a server that
**has no internet access** (typical for an Iranian VPS behind strict
filtering).

## The three files you transfer to the offline server

| File | Purpose |
|------|---------|
| `x-ui-linux-amd64.tar.gz` | Full bundle: the `x-ui` panel binary (statically linked, no glibc/musl runtime surprises) + `xray-linux-amd64` core + `geoip*.dat` / `geosite*.dat` + systemd unit files (`x-ui.service.debian/arch/rhel`). Includes the **quota-exhaust disconnect fix** (commit `1cd06dd`). |
| `install_offline.sh` | Offline installer. Performs **zero** network calls. Located in the repo root at `/install_offline.sh`. |
| `x-ui.sh` | The admin / management CLI. Located in the repo root at `/x-ui.sh`. |

> Only **amd64** (x86_64) is pre-built. If your VPS is `arm64` / `armv7` / etc,
> rebuild the tarball on any internet-connected machine with the helper
> script documented at the bottom of this file.

## Install steps

1. On a machine that **has** internet access, clone your fork or just
   download the three files:
   ```bash
   git clone -b main https://github.com/admin6501/3x-ui
   cd 3x-ui
   ls offline/x-ui-linux-amd64.tar.gz install_offline.sh x-ui.sh
   ```

2. Copy the three files to the offline Iranian server via `scp`,
   a USB stick, or any out-of-band channel:
   ```bash
   scp offline/x-ui-linux-amd64.tar.gz install_offline.sh x-ui.sh \
       root@<iran-server-ip>:/root/
   ```

3. SSH into the Iran server and run:
   ```bash
   cd /root
   chmod +x install_offline.sh
   sudo ./install_offline.sh
   ```
   The installer auto-detects architecture, extracts into
   `/usr/local/x-ui/`, installs the admin CLI to `/usr/bin/x-ui`, drops the
   correct systemd unit into `/etc/systemd/system/x-ui.service`, and starts
   the service. It then prints randomly-generated admin credentials and the
   access URL.

4. Open the printed URL (`http://<server-ip>:<port>/<path>`) and log in.

## Host requirements (offline)

The installer does **not** install any packages. Your offline VPS must
already have these common utilities available on `PATH`:

* `tar`, `awk`, `grep`, `sed` (coreutils — always present)
* `systemctl` on systemd distros (Debian/Ubuntu/RHEL/Alma/Rocky/Fedora/CentOS/Arch)
* `rc-update` / `rc-service` on Alpine/OpenRC

All of these ship with a stock install of every supported distro, so in
practice you don't need to do anything. The installer will detect and
report any that are missing.

**SSL note**: offline installs skip `acme.sh` / Let's Encrypt because both
require outbound internet to port 80. You can enable HTTPS later from the
`x-ui` admin menu once the host has DNS + port 80 egress, or by dropping
your own pre-issued cert under `/root/cert/` and setting its path via the
panel.

## Re-creating the bundle for a non-amd64 arch

Run this on any online Linux host with Go 1.26+ and the appropriate
cross-toolchain installed:

```bash
# amd64 (example)
export ARCH=amd64
export CGO_ENABLED=1 GOOS=linux GOARCH=$ARCH
export CC=x86_64-linux-gnu-gcc        # or a bootlin musl toolchain
go build -ldflags "-w -s -linkmode external -extldflags '-static'" \
         -o dist/x-ui main.go
```

Then fetch `xray-linux-<ARCH>` and the geo .dat files (the exact URLs are
already in `.github/workflows/release.yml` — copy from there), arrange
them in the layout below, and `tar -zcf x-ui-linux-<ARCH>.tar.gz x-ui/`:

```
x-ui/
├── x-ui                    # the panel binary, +x
├── x-ui.sh                 # admin CLI (same file as /x-ui.sh in repo root)
├── x-ui.service.debian
├── x-ui.service.arch
├── x-ui.service.rhel
└── bin/
    ├── xray-linux-<ARCH>   # +x
    ├── geoip.dat
    ├── geosite.dat
    ├── geoip_IR.dat
    ├── geosite_IR.dat
    ├── geoip_RU.dat
    └── geosite_RU.dat
```

## What's inside the binary
- Upstream 3x-ui at commit `12c10db` (`feat(custom-geo): refresh index UI`),
- PLUS commit `1cd06dd` — **fix(traffic-job): drop active sessions of
  auto-disabled clients** (the quota-exhaust bug fix this fork was created
  for).
