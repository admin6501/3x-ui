#!/bin/bash
#
# 3x-ui  —  OFFLINE installer (no internet required on target host).
#
# Ship THREE files to the target server (any path is fine — e.g. /root):
#
#   1) x-ui-linux-<arch>.tar.gz   full 3x-ui bundle (x-ui binary + xray +
#                                 geo dat files + systemd units)
#   2) install_offline.sh         this script
#   3) x-ui.sh                    the admin/management CLI (optional here —
#                                 if missing on disk, this script will fall
#                                 back to the copy bundled inside the tarball)
#
# Then on the server:
#
#   chmod +x install_offline.sh
#   sudo ./install_offline.sh            # auto-detects the .tar.gz in $PWD
#   # or, explicit tarball path:
#   sudo ./install_offline.sh /path/to/x-ui-linux-amd64.tar.gz
#
# This script performs ZERO network calls for the x-ui bundle itself
# (binary + xray + geo dat files come from the tarball you ship with it).
# Base OS dependencies (curl, tar, cron, socat, openssl, ca-certificates,
# tzdata) ARE installed automatically via apt/dnf/pacman/zypper/apk —
# this matches install.sh's behaviour so the offline installer doesn't
# leave a half-working panel on a fresh server. If your target host has
# no package-manager network, set OFFLINE_SKIP_DEPS=1 to skip this step.
#

set -u

red='\033[0;31m'
green='\033[0;32m'
blue='\033[0;34m'
yellow='\033[0;33m'
plain='\033[0m'

xui_folder="${XUI_MAIN_FOLDER:=/usr/local/x-ui}"
xui_service_dir="${XUI_SERVICE:=/etc/systemd/system}"

[[ $EUID -ne 0 ]] && echo -e "${red}Fatal:${plain} please run as root." >&2 && exit 1

# -------- OS detection -------- #
if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC1091
    source /etc/os-release
    release=${ID:-unknown}
elif [[ -f /usr/lib/os-release ]]; then
    # shellcheck disable=SC1091
    source /usr/lib/os-release
    release=${ID:-unknown}
else
    echo -e "${red}Failed to detect OS.${plain}" >&2
    exit 1
fi
echo -e "${blue}[offline]${plain} OS release: ${release}"

# -------- Install base prerequisites (mirrors install.sh::install_base) -------- #
# These are the same packages the online installer (install.sh) pulls in:
# cron/cronie/dcron, curl, tar, tzdata/timezone, socat, ca-certificates,
# openssl. Without them, acme.sh / time-zone handling / certificate
# verification all fail silently on a fresh minimal image. Skip with
# OFFLINE_SKIP_DEPS=1 if the target host genuinely has no package mirror.
install_base() {
    if [[ "${OFFLINE_SKIP_DEPS:-0}" == "1" ]]; then
        echo -e "${yellow}[offline]${plain} OFFLINE_SKIP_DEPS=1 set; skipping base package install."
        return 0
    fi
    echo -e "${blue}[offline]${plain} Installing base prerequisites via the system package manager..."
    case "${release}" in
        ubuntu | debian | armbian)
            apt-get update -y && apt-get install -y -q cron curl tar tzdata socat ca-certificates openssl
            ;;
        fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol)
            dnf -y update && dnf install -y -q cronie curl tar tzdata socat ca-certificates openssl
            ;;
        centos)
            if [[ "${VERSION_ID:-}" =~ ^7 ]]; then
                yum -y update && yum install -y cronie curl tar tzdata socat ca-certificates openssl
            else
                dnf -y update && dnf install -y -q cronie curl tar tzdata socat ca-certificates openssl
            fi
            ;;
        arch | manjaro | parch)
            pacman -Syu --noconfirm cronie curl tar tzdata socat ca-certificates openssl
            ;;
        opensuse-tumbleweed | opensuse-leap)
            zypper refresh && zypper -q install -y cron curl tar timezone socat ca-certificates openssl
            ;;
        alpine)
            apk update && apk add dcron curl tar tzdata socat ca-certificates openssl
            ;;
        *)
            # Unknown distros default to apt; if that fails, the missing-tool
            # check below will surface a clear actionable error.
            apt-get update -y && apt-get install -y -q cron curl tar tzdata socat ca-certificates openssl
            ;;
    esac
    local rc=$?
    if [[ $rc -ne 0 ]]; then
        echo -e "${yellow}[offline]${plain} Base package install reported errors (rc=$rc)."
        echo -e "${yellow}[offline]${plain} If your host has no package-manager network, rerun with OFFLINE_SKIP_DEPS=1"
        echo -e "${yellow}[offline]${plain} after installing the prerequisites from your offline mirror."
    fi
    return 0
}

install_base

arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64)         echo 'amd64' ;;
        i*86 | x86)                   echo '386' ;;
        armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
        armv7* | armv7 | arm)         echo 'armv7' ;;
        armv6* | armv6)               echo 'armv6' ;;
        armv5* | armv5)               echo 'armv5' ;;
        s390x)                        echo 's390x' ;;
        *) echo -e "${red}Unsupported CPU architecture: $(uname -m)${plain}" >&2; exit 1 ;;
    esac
}
ARCH=$(arch)
echo -e "${blue}[offline]${plain} Arch:       ${ARCH}"

# -------- Locate tarball -------- #
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

if [[ $# -ge 1 && -n "${1:-}" ]]; then
    bundle="$1"
else
    bundle="${script_dir}/x-ui-linux-${ARCH}.tar.gz"
    if [[ ! -f "$bundle" ]]; then
        # fallback: first match in cwd / script dir
        bundle=$(ls -1 "${script_dir}"/x-ui-linux-*.tar.gz 2>/dev/null | head -n1 || true)
        [[ -z "$bundle" ]] && bundle=$(ls -1 "$(pwd)"/x-ui-linux-*.tar.gz 2>/dev/null | head -n1 || true)
    fi
fi

if [[ -z "${bundle:-}" || ! -f "$bundle" ]]; then
    echo -e "${red}Fatal:${plain} could not find x-ui-linux-${ARCH}.tar.gz near this script."
    echo -e "       Place the tarball next to install_offline.sh, or pass its path as an argument."
    exit 1
fi
echo -e "${blue}[offline]${plain} Bundle:     ${bundle}"

# -------- Verify required host tools (no internet fallback!) -------- #
missing=()
for bin in tar awk grep sed systemctl; do
    command -v "$bin" >/dev/null 2>&1 || missing+=("$bin")
done
if [[ $release == "alpine" ]]; then
    # alpine uses openrc, not systemd
    missing=("${missing[@]/systemctl}")
    command -v rc-update >/dev/null 2>&1 || missing+=("openrc")
fi
if [[ ${#missing[@]} -gt 0 ]]; then
    echo -e "${red}Fatal:${plain} missing required tools on this host: ${missing[*]}"
    echo -e "       Install them from your offline package mirror and rerun."
    exit 1
fi

# -------- Stop any existing x-ui service -------- #
if [[ $release == "alpine" ]]; then
    rc-service x-ui stop > /dev/null 2>&1 || true
else
    systemctl stop x-ui > /dev/null 2>&1 || true
fi

# -------- Extract into /usr/local -------- #
install_root=$(dirname "$xui_folder")       # normally /usr/local
mkdir -p "$install_root"

# Remove old install dir (but preserve /etc/x-ui DB)
if [[ -d "$xui_folder" ]]; then
    echo -e "${yellow}[offline]${plain} Removing previous install at $xui_folder (DB in /etc/x-ui is kept)."
    rm -rf "$xui_folder"
fi

echo -e "${blue}[offline]${plain} Extracting bundle..."
tar -xzf "$bundle" -C "$install_root"
if [[ ! -d "$install_root/x-ui" ]]; then
    echo -e "${red}Fatal:${plain} tarball did not produce an 'x-ui/' directory inside $install_root."
    exit 1
fi

cd "$xui_folder" || { echo -e "${red}cd $xui_folder failed${plain}"; exit 1; }
chmod +x x-ui x-ui.sh 2>/dev/null || true

# Rename armv{5,6,7} xray binary to the generic 'arm' suffix the code expects
if [[ "$ARCH" == "armv5" || "$ARCH" == "armv6" || "$ARCH" == "armv7" ]]; then
    if [[ -f "bin/xray-linux-${ARCH}" ]]; then
        mv -f "bin/xray-linux-${ARCH}" bin/xray-linux-arm
        chmod +x bin/xray-linux-arm
    fi
fi
[[ -f "bin/xray-linux-${ARCH}" ]] && chmod +x "bin/xray-linux-${ARCH}"

# -------- Install x-ui admin CLI -------- #
# Prefer the copy the user uploaded next to the installer; fall back to the
# one bundled inside the tarball.
admin_cli_src=""
if [[ -f "${script_dir}/x-ui.sh" ]]; then
    admin_cli_src="${script_dir}/x-ui.sh"
elif [[ -f "${xui_folder}/x-ui.sh" ]]; then
    admin_cli_src="${xui_folder}/x-ui.sh"
fi
if [[ -z "$admin_cli_src" ]]; then
    echo -e "${red}Fatal:${plain} x-ui.sh not found — neither next to this installer nor inside the tarball."
    exit 1
fi
install -m 0755 "$admin_cli_src" /usr/bin/x-ui
echo -e "${green}[offline]${plain} Installed admin CLI -> /usr/bin/x-ui"

mkdir -p /var/log/x-ui

# -------- Install service unit (offline, from tarball only) -------- #
if [[ $release == "alpine" ]]; then
    if [[ ! -f "${xui_folder}/x-ui.rc" ]]; then
        # upstream release tarball does not include x-ui.rc; fall back to the
        # copy shipped in the repo alongside install_offline.sh (script_dir).
        if [[ -f "${script_dir}/x-ui.rc" ]]; then
            install -m 0755 "${script_dir}/x-ui.rc" /etc/init.d/x-ui
        else
            echo -e "${red}Fatal:${plain} x-ui.rc not found for Alpine/OpenRC install."
            exit 1
        fi
    else
        install -m 0755 "${xui_folder}/x-ui.rc" /etc/init.d/x-ui
    fi
    rc-update add x-ui default > /dev/null 2>&1
    rc-service x-ui start
else
    case "${release}" in
        ubuntu | debian | armbian)           unit_src="x-ui.service.debian" ;;
        arch | manjaro | parch)              unit_src="x-ui.service.arch" ;;
        *)                                   unit_src="x-ui.service.rhel" ;;
    esac
    if [[ ! -f "${xui_folder}/${unit_src}" ]]; then
        echo -e "${red}Fatal:${plain} ${unit_src} not found in tarball (${xui_folder})."
        exit 1
    fi
    install -m 0644 "${xui_folder}/${unit_src}" "${xui_service_dir}/x-ui.service"
    chown root:root "${xui_service_dir}/x-ui.service" 2>/dev/null || true
    systemctl daemon-reload
    systemctl enable x-ui > /dev/null 2>&1
    systemctl start x-ui
fi

# -------- First-run credential setup (no SSL / no acme — pure offline) -------- #
gen_random_string() {
    local length="$1"
    head -c $((length * 4)) /dev/urandom | tr -dc 'a-zA-Z0-9' | head -c "$length"
}

# Only (re)generate if panel has never been configured on this host.
if [[ ! -f /etc/x-ui/x-ui.db ]]; then
    user=$(gen_random_string 10)
    pass=$(gen_random_string 14)
    port=$(shuf -i 20000-62000 -n 1 2>/dev/null || echo 54321)
    webpath=$(gen_random_string 18)

    "${xui_folder}/x-ui" setting \
        -username "$user" \
        -password "$pass" \
        -port "$port" \
        -webBasePath "$webpath" > /dev/null

    # Restart so new settings take effect
    if [[ $release == "alpine" ]]; then
        rc-service x-ui restart > /dev/null 2>&1 || true
    else
        systemctl restart x-ui > /dev/null 2>&1 || true
    fi

    server_ip=$(ip -4 addr show scope global 2>/dev/null | awk '/inet /{split($2,a,"/"); print a[1]; exit}')
    [[ -z "$server_ip" ]] && server_ip="<server-ip>"

    echo -e ""
    echo -e "${green}═══════════════════════════════════════════${plain}"
    echo -e "${green}     3x-ui offline install complete       ${plain}"
    echo -e "${green}═══════════════════════════════════════════${plain}"
    echo -e "${green}Username    :${plain} $user"
    echo -e "${green}Password    :${plain} $pass"
    echo -e "${green}Port        :${plain} $port"
    echo -e "${green}WebBasePath :${plain} $webpath"
    echo -e "${green}Access URL  :${plain} http://${server_ip}:${port}/${webpath}"
    echo -e "${green}═══════════════════════════════════════════${plain}"
    echo -e "${yellow}⚠ Save these credentials somewhere safe.${plain}"
    echo -e "${yellow}⚠ SSL/acme was skipped (no internet). Configure HTTPS${plain}"
    echo -e "${yellow}   later via ${blue}x-ui${yellow} if/when the server has DNS + :80 outbound.${plain}"
else
    echo -e ""
    echo -e "${green}═══════════════════════════════════════════${plain}"
    echo -e "${green}     3x-ui binary upgraded / re-installed ${plain}"
    echo -e "${green}═══════════════════════════════════════════${plain}"
    echo -e "${yellow}Existing /etc/x-ui/x-ui.db preserved — your previous"
    echo -e "${yellow}credentials, inbounds and clients are unchanged.${plain}"
fi

# Migrate DB schema in case this bundle is newer than the on-disk DB
"${xui_folder}/x-ui" migrate > /dev/null 2>&1 || true

echo -e ""
echo -e "Run ${blue}x-ui${plain} for the admin menu, or ${blue}x-ui status${plain} to verify the service."
