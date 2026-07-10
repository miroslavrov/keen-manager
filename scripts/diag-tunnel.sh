#!/bin/sh
# keen-manager — tunnel diagnostic v3 (session 15).
#
# WHY (session-15 pivot): the SAME vless-reality-vision config works on the
# user's PC and phone on the SAME LAN — only the router fails. That rules out a
# general DPI block and the server (they'd kill the PC/phone too) and points at
# a router-LOCAL cause. Prime suspect: MSS/MTU. LAN traffic FORWARDED through the
# router is MSS-clamped-to-PMTU by KeeneticOS, but Xray's OWN egress to the
# server is a router-local socket (OUTPUT chain) that is NOT clamped — so on a
# reduced-MTU / TSPU WAN the small reality handshake gets through while full-size
# data segments blackhole. Symptom: "tunnel establishes, payload never flows".
#
# WHAT THIS PROVES: it runs the tunnel outbound three+ ways through a local
# SOCKS and shows which one carries traffic:
#   - xray-plain    : sockopt as keen-manager ships it (mark 255, no clamp)   [reproduces the failure]
#   - xray-mss1380  : + tcpMaxSeg 1380  (keen-manager's DEFAULT clamp)        [validates the shipped fix]
#   - xray-mss1280  : + tcpMaxSeg 1280  (aggressive fallback)                 [brackets the needed value]
# If a clamped variant carries traffic while xray-plain does not, MSS/MTU is
# CONFIRMED and the fix is proven — set it on the Settings page (MSS clamp).
#
# SAFETY: read-only. Does NOT touch the keen-manager service, iptables, ip rules
# or routes. Spawns short-lived xray on 10810-10813 (self-terminating watchdog,
# no dependency on `timeout` — busybox may not ship it). Run with the blanc
# tunnel OFF (normal internet, :10808 free); the script uses its own ports.
#
# Usage (pin to a commit so the CDN can't serve a stale copy):
#   curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/diag-tunnel.sh | sh

CFG="${KEEN_XRAY_CONFIG:-/opt/etc/keen-manager/xray/config.json}"
XRAY="${XRAY_BIN:-/opt/sbin/xray}"
TMP="${KEEN_TMP:-/opt/tmp}"
mkdir -p "$TMP"

echo "=================== keen-manager tunnel diag v3 ==================="
[ -x "$XRAY" ] || { echo "!! no xray at $XRAY"; exit 1; }
"$XRAY" -version 2>/dev/null | head -1
[ -f "$CFG" ] || { echo "!! no config at $CFG — activate a server once so it is generated"; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "!! jq missing (opkg install jq)"; exit 1; }
echo

# ---- server endpoint + reality SNI (from live config) ----------------------
EP=$(jq -r '(.outbounds[]|select(.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks")|(.settings.vnext//.settings.servers)[0]|"\(.address) \(.port)")' "$CFG" 2>/dev/null | head -1)
H=$(echo "$EP" | awk '{print $1}'); P=$(echo "$EP" | awk '{print $2}')
SNI=$(jq -r '[.outbounds[]|select(.protocol=="vless")|.streamSettings.realitySettings.serverName//.streamSettings.tlsSettings.serverName]|map(select(.!=null))[0]//""' "$CFG" 2>/dev/null)
echo "server endpoint : $H:$P"
echo "reality SNI     : $SNI"
printf "direct TCP      : "
curl -s --connect-timeout 6 -o /dev/null -w 'time_connect=%{time_connect}s http_code=%{http_code}\n' "https://$H:$P/" 2>&1
echo

# ---- WAN interface + its MTU (context for the MSS story) -------------------
echo "=== WAN / MTU context ==="
WANDEV=$(ip route show default 2>/dev/null | sed -n 's/.* dev \([^ ]*\).*/\1/p' | head -1)
echo "  default route dev : ${WANDEV:-?}"
if [ -n "$WANDEV" ]; then
	ip link show "$WANDEV" 2>/dev/null | grep -o 'mtu [0-9]*' | sed 's/^/  WAN /'
fi
# Best-effort PMTU-to-server probe via DF ping. ICMP is often filtered and
# busybox ping may lack -M, so treat any result as a HINT, not proof. The
# decisive signal is the clamped-xray test below.
echo "  PMTU probe to $H (DF ping; best-effort — ICMP may be blocked):"
if ping -c1 -W2 -s 1000 "$H" >/dev/null 2>&1; then
	for PL in 1472 1412 1372 1352 1272 1232; do   # IP MTU = payload + 28
		if ping -c1 -W2 -M do -s "$PL" "$H" >/dev/null 2>&1; then
			echo "    ok  payload=$PL  (IP MTU $((PL+28)) passes unfragmented)"
		else
			echo "    --  payload=$PL  (IP MTU $((PL+28)) blocked/fragmented)"
		fi
	done
else
	echo "    (server does not answer ICMP, or -M unsupported — skipping; rely on the xray test)"
fi
echo

# ---- independent TLS/DPI probe: real TLS ClientHello (SNI) to the server ----
# A reality server proxies a genuine ClientHello for its SNI to the real dest,
# so a CLEAN path completes the handshake. A reset/timeout here = DPI on that
# SNI/dest. NOTE: session-15 evidence (PC/phone work on this LAN) already argues
# against a DPI block; kept for completeness.
echo "=== TLS/DPI probe (SNI $SNI -> $H:$P) ==="
if [ -n "$SNI" ]; then
	curl -sv -k -m 10 --connect-to "$SNI:443:$H:$P" "https://$SNI/" -o /dev/null 2>&1 \
	  | grep -iE 'connected|SSL connection|TLS|handshake|reset|timed out|refused|certificate|subject:|issuer:|HTTP/' | sed 's/^/  /'
else
	echo "  (no SNI parsed — skipping)"
fi
echo

# ---- build SOCKS-only debug configs (jq without test()/regex) --------------
# $s = tag of the first real (server) outbound. Each config keeps ONLY that
# outbound + a fresh SOCKS inbound, forces debug logging, and varies the
# outbound sockopt so we can isolate the MSS/MTU effect.
build() {   # build OUTFILE PORT SOCKOPT_JSON
	jq --argjson so "$3" \
	  '(.outbounds|map(select(.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks"))[0].tag) as $s
	   | {log:{loglevel:"debug"},
	      inbounds:[{tag:"socks-in",listen:"127.0.0.1",port:'"$2"',protocol:"socks",settings:{auth:"noauth","udp":true}}],
	      outbounds:[.outbounds[]
	        | if (.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks")
	          then (.streamSettings.sockopt = ((.streamSettings.sockopt//{}) + $so)) else . end],
	      routing:{rules:[{type:"field",inboundTag:["socks-in"],outboundTag:$s}]}}' \
	  "$CFG" > "$1"
}
build "$TMP/xray-plain.json"   10810 '{"mark":255}'
build "$TMP/xray-mss1380.json" 10811 '{"mark":255,"tcpMaxSeg":1380}'
build "$TMP/xray-mss1280.json" 10812 '{"mark":255,"tcpMaxSeg":1280}'

# portable bounded run: background xray + a self-terminating watchdog (no
# dependency on `timeout`, which busybox may not provide).
run_test() {
	NAME="$1"; CONF="$2"; PORT="$3"
	echo "----------- $NAME (socks 127.0.0.1:$PORT) -----------"
	if [ ! -s "$CONF" ]; then echo "  !! config build failed ($CONF empty)"; return; fi
	echo "  xray -test:"; "$XRAY" -test -config "$CONF" 2>&1 | tail -2 | sed 's/^/    /'
	"$XRAY" run -config "$CONF" > "$TMP/$NAME.log" 2>&1 &
	XPID=$!
	( sleep 14; kill "$XPID" 2>/dev/null ) &
	WPID=$!
	sleep 2
	echo "  listening on :$PORT ?"; { ss -ltn 2>/dev/null || netstat -ltn 2>/dev/null; } | grep ":$PORT " | sed 's/^/    /'
	printf "  SOCKS exit-IP (want the SERVER ip): "
	curl -s -x socks5h://127.0.0.1:"$PORT" -m 10 https://api.ipify.org; echo
	curl -s -x socks5h://127.0.0.1:"$PORT" -m 10 -o /dev/null -w '  gstatic=%{http_code}\n' https://www.gstatic.com/generate_204
	# a slightly larger fetch shakes out MTU-only failures a tiny GET can hide
	curl -s -x socks5h://127.0.0.1:"$PORT" -m 12 -o /dev/null -w '  1KB-fetch=%{http_code} size=%{size_download}\n' https://www.gstatic.com/generate_204
	sleep 1
	kill "$XPID" 2>/dev/null; kill "$WPID" 2>/dev/null; wait "$XPID" 2>/dev/null
	echo "  --- last xray debug lines ---"
	if [ -s "$TMP/$NAME.log" ]; then tail -8 "$TMP/$NAME.log" | sed 's/^/    /'; else echo "    (log empty)"; fi
	echo
}

echo "=================== decisive standalone test ==================="
run_test xray-plain   "$TMP/xray-plain.json"   10810
run_test xray-mss1380 "$TMP/xray-mss1380.json" 10811
run_test xray-mss1280 "$TMP/xray-mss1280.json" 10812

echo "=================== how to read it ============================"
echo "  A clamped variant (mss1380/mss1280) carries traffic but xray-plain does NOT"
echo "      -> MSS/MTU CONFIRMED. Set the MSS clamp on the Settings page:"
echo "         keep 1380 (default) if mss1380 worked, else use the lowest that worked."
echo "  ALL THREE carry the SERVER ip                 -> outbound is fine; bug is the TPROXY LAN capture"
echo "  ALL THREE empty + log 'reset'                 -> DPI on the handshake (unlikely: PC/phone work here)"
echo "  ALL THREE empty + log 'i/o timeout'/'dial'    -> path to server blocked mid-handshake"
echo "  log 'REALITY'/'invalid'                       -> reality params (pbk/sid/sni) mismatch with server"
echo "==============================================================="
