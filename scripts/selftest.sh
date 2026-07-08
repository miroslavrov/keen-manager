#!/bin/sh
# keen-manager self-test — safe, read-only diagnostic for a Keenetic router.
#
#   sh /opt/etc/keen-manager/selftest.sh        # (or pipe from curl)
#   curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/selftest.sh | sh
#
# What it does (and does NOT do):
#   * Prints device facts (arch, KeeneticOS version, installed components).
#   * Runs keen-manager as a DAEMON in DRY-RUN mode (KEEN_DRY_RUN=1) on a temp
#     port with a temp data dir, then exercises the local API over 127.0.0.1.
#   * DRY-RUN means every device-mutating command (awg-quick, xray, iptables,
#     ip, nfqws2) is a NO-OP — this test cannot change routing, firewall, or any
#     service. It only checks that the binary, API, parsing and UI work.
#   * Cleans up after itself. Your real config under /opt/etc/keen-manager is
#     untouched (the test uses its own throwaway data dir).
#
# Paste the full output back to share the result.

BIN="${BIN:-/opt/bin/keen-manager}"
PORT="${PORT:-18088}"
TMP="/tmp/keen-selftest.$$"
BASE="http://127.0.0.1:${PORT}"

line() { echo "------------------------------------------------------------"; }
say()  { echo "[selftest] $*"; }

# --- download helper --------------------------------------------------------
DL=""
command -v curl >/dev/null 2>&1 && DL="curl -fsS"
[ -z "$DL" ] && command -v wget >/dev/null 2>&1 && DL="wget -qO-"

get() { # get <path>
	if command -v curl >/dev/null 2>&1; then curl -fsS "${BASE}$1" 2>/dev/null; else wget -qO- "${BASE}$1" 2>/dev/null; fi
}
post() { # post <path> <json>
	if command -v curl >/dev/null 2>&1; then
		curl -fsS -X POST "${BASE}$1" -H 'Content-Type: application/json' -d "$2" 2>/dev/null
	else
		wget -qO- --header='Content-Type: application/json' --post-data="$2" "${BASE}$1" 2>/dev/null
	fi
}

line; say "1) Device facts"; line
echo "date:        $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "uname:       $(uname -a 2>/dev/null)"
echo "uname -m:    $(uname -m 2>/dev/null)"
if command -v opkg >/dev/null 2>&1; then
	echo "opkg arch:   $(opkg print-architecture 2>/dev/null | awk '$1=="arch"&&$2!="all"&&$2!="noarch"{print $2}' | tr '\n' ' ')"
fi
if command -v ndmc >/dev/null 2>&1; then
	echo "keenetic:    $(ndmc -c 'show version' 2>/dev/null | awk -F': ' '/title:/{print $2}' | head -n1)"
	echo "device:      $(ndmc -c 'show version' 2>/dev/null | awk -F': ' '/device:/{print $2}' | head -n1)"
fi

line; say "2) Installed components (drivers this orchestrator relies on)"; line
for probe in \
	"awg:$(command -v awg 2>/dev/null || echo /opt/sbin/awg)" \
	"awg-quick:$(command -v awg-quick 2>/dev/null || echo /opt/bin/awg-quick)" \
	"xray:$(command -v xray 2>/dev/null || echo /opt/sbin/xray)" \
	"nfqws2:$(command -v nfqws2 2>/dev/null || echo /opt/usr/bin/nfqws2)" \
	"iptables:$(command -v iptables 2>/dev/null || echo /opt/sbin/iptables)" \
	"ip(full):$(command -v ip 2>/dev/null || echo /opt/sbin/ip)"; do
	name=${probe%%:*}; path=${probe#*:}
	if [ -n "$path" ] && [ -x "$path" ]; then echo "  [ok]   $name  -> $path"; else echo "  [MISS] $name  (not found)"; fi
done
# AmneziaWG kernel module presence (any of the usual locations)
if find /lib/modules /lib/system-modules -name 'amneziawg*.ko*' 2>/dev/null | grep -q .; then
	echo "  [ok]   amneziawg kernel module present"
else
	echo "  [MISS] amneziawg kernel module (needed for AWG tunnels)"
fi

line; say "3) keen-manager binary"; line
if [ ! -x "$BIN" ]; then
	say "binary not found at $BIN — install it first (scripts/install.sh) or set BIN=..."
	exit 1
fi
"$BIN" version

line; say "4) API smoke test (DRY-RUN — no device changes)"; line
mkdir -p "$TMP"
KEEN_DRY_RUN=1 KEEN_DATA_DIR="$TMP" KEEN_LISTEN="127.0.0.1:${PORT}" "$BIN" daemon >"$TMP/daemon.log" 2>&1 &
DPID=$!
# Wait for it to answer.
i=0; while [ "$i" -lt 15 ]; do get /api/health >/dev/null 2>&1 && break; i=$((i+1)); sleep 1; done

echo "health:   $(get /api/health)"
echo "state:    $(get /api/state | cut -c1-200)"
echo "nfqws:    $(get /api/nfqws)"
# Parse a sample vless+reality link (no secrets of yours involved).
SAMPLE='vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0&sid=0123abcd&type=tcp&flow=xtls-rprx-vision#SelfTest'
echo "add xray: $(post /api/connections "{\"type\":\"xray\",\"name\":\"selftest\",\"share_link\":\"$SAMPLE\"}")"
echo "conns:    $(get /api/connections | cut -c1-200)"
echo "ui bytes: $(get / | wc -c) (index.html served)"

kill "$DPID" 2>/dev/null
rm -rf "$TMP"
line; say "done. If steps 1-4 look sane, paste this whole output back."
