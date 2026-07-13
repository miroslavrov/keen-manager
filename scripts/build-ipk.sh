#!/bin/sh
# build-ipk.sh — build Entware/OPKG .ipk packages for keen-manager.
#
# An .ipk is an ar archive containing:
#   debian-binary    — version string "2.0"
#   control.tar.gz   — control file + postinst/prerm scripts
#   data.tar.gz      — payload (binary + init script + hook)
#
# Usage: ./scripts/build-ipk.sh <version> <build-dir>
#   version: e.g. 0.1.0-rc.12 (without leading 'v')
#   build-dir: directory containing keen-manager-<arch> binaries
#
# Outputs: <build-dir>/keen-manager_<version>_<arch>.ipk

set -e

VERSION="${1:?usage: build-ipk.sh <version> <build-dir>}"
BUILD_DIR="${2:-build}"
VERSION="${VERSION#v}"  # strip leading 'v' if present

# Map Go arch names to Entware opkg architecture names.
arch_to_ipk() {
    case "$1" in
        arm64)  echo "aarch64" ;;
        arm)    echo "armv7" ;;
        mipsle) echo "mipsel" ;;
        mips)   echo "mips" ;;
        *)      echo "" ;;
    esac
}

for suffix in mipsle mips arm64 arm; do
    bin="${BUILD_DIR}/keen-manager-${suffix}"
    if [ ! -f "$bin" ]; then
        echo "skip ${suffix}: binary not found"
        continue
    fi

    ipk_arch=$(arch_to_ipk "$suffix")
    if [ -z "$ipk_arch" ]; then
        echo "skip ${suffix}: no IPK arch mapping"
        continue
    fi

    pkgname="keen-manager_${VERSION}_${ipk_arch}.ipk"
    workdir="${BUILD_DIR}/ipk-${suffix}"
    rm -rf "$workdir"
    mkdir -p "$workdir"

    # --- data.tar.gz ---
    data_root="${workdir}/data"
    mkdir -p "${data_root}/opt/bin" "${data_root}/opt/etc/init.d" "${data_root}/opt/etc/ndm/netfilter.d"
    cp "$bin" "${data_root}/opt/bin/keen-manager"
    chmod 0755 "${data_root}/opt/bin/keen-manager"
    cp scripts/init.d/S99keen-manager "${data_root}/opt/etc/init.d/S99keen-manager"
    chmod 0755 "${data_root}/opt/etc/init.d/S99keen-manager"
    # ndm hook (content written by `keen-manager install-hook`, but we pre-install it)
    cat > "${data_root}/opt/etc/ndm/netfilter.d/50-keen-manager" <<'HOOK'
#!/bin/sh
[ "$type" = "iptables" ] || exit 0
[ "$table" = "mangle" ] || exit 0
/opt/bin/keen-manager route reapply >/dev/null 2>&1 &
exit 0
HOOK
    chmod 0755 "${data_root}/opt/etc/ndm/netfilter.d/50-keen-manager"

    (cd "$workdir" && tar czf data.tar.gz -C data .)

    # --- control.tar.gz ---
    ctrl_root="${workdir}/control"
    mkdir -p "$ctrl_root"

    cat > "${ctrl_root}/control" <<EOF
Package: keen-manager
Version: ${VERSION}
Architecture: ${ipk_arch}
Maintainer: miroslavrov
Section: net
Priority: optional
Depends: curl
Description: Unified VPN (Xray/AmneziaWG) + DPI-bypass manager for Keenetic routers.
EOF

    cat > "${ctrl_root}/postinst" <<'POSTINST'
#!/bin/sh
/opt/etc/init.d/S99keen-manager enable 2>/dev/null || true
/opt/etc/init.d/S99keen-manager start 2>/dev/null || true
exit 0
POSTINST
    chmod 0755 "${ctrl_root}/postinst"

    cat > "${ctrl_root}/prerm" <<'PRERM'
#!/bin/sh
/opt/etc/init.d/S99keen-manager stop 2>/dev/null || true
exit 0
PRERM
    chmod 0755 "${ctrl_root}/prerm"

    cat > "${ctrl_root}/conffiles" <<'CONF'
/opt/etc/keen-manager/state.json
CONF

    (cd "$workdir" && tar czf control.tar.gz -C control .)

    # --- debian-binary ---
    echo "2.0" > "${workdir}/debian-binary"

    # --- assemble .ipk (ar archive) ---
    (cd "$workdir" && ar rcs "${BUILD_DIR}/${pkgname}" debian-binary control.tar.gz data.tar.gz)

    echo "built ${pkgname}"
    rm -rf "$workdir"
done
