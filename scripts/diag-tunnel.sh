#!/bin/sh
# keen-manager — tunnel diagnostic v2 (session 14).
#
# WHY: `xray -test` passes and Xray listens on 127.0.0.1:10808, but the SOCKS
# tunnel carries nothing and the whole router loses internet under TPROXY. TCP
# to the server endpoint succeeds, and BOTH SO_MARK-stripped and SO_MARK-255
# variants fail identically -> the reality/vless OUTBOUND handshake is what
# dies, not routing or the mark. This build reliably captures the xray DEBUG
# log (bounded foreground run + full cat, not a backgrounded tail that loses
# buffered output) and adds an independent TLS/DPI probe to the reality dest,
# so we can tell a DPI reset from a config/handshake error.
#
# SAFETY: read-only. Does NOT touch the keen-manager service, iptables, ip
# rules or routes. Spawns short-lived xray on 10810/10811 (bounded by `timeout`)
# and probes through their SOCKS. Run with the blanc tunnel OFF (normal internet,
# :10808 free); the script uses its own ports regardless.
#
# Usage (pin to a commit so the CDN can't serve a stale copy):
#   curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/diag-tunnel.sh | sh

CFG="${KEEN_XRAY_CONFIG:-/opt/etc/keen-manager/xray/config.json}"
XRAY="${XRAY_BIN:-/opt/sbin/xray}"
TMP="${KEEN_TMP:-/opt/tmp}"
mkdir -p "$TMP"

echo "=================== keen-manager tunnel diag v2 ==================="
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

# ---- independent TLS/DPI probe: real TLS ClientHello (SNI) to the server ----
# A reality server proxies a genuine ClientHello for its SNI to the real dest,
# so a CLEAN path completes the handshake. A reset/timeout here = DPI on that
# SNI/dest (i.e. reality's cover is being cut), which no xray tweak can fix.
echo "=== TLS/DPI probe (SNI $SNI -> $H:$P) ==="
if [ -n "$SNI" ]; then
	curl -sv -k -m 10 --connect-to "$SNI:443:$H:$P" "https://$SNI/" -o /dev/null 2>&1 \
	  | grep -iE 'connected|SSL connection|TLS|handshake|reset|timed out|refused|certificate|subject:|issuer:|HTTP/' | sed 's/^/  /'
else
	echo "  (no SNI parsed — skipping)"
fi
echo

# ---- build two SOCKS-only debug configs (jq without test()/regex) ----------
jq '(.outbounds|map(select(.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks"))[0].tag) as $s | {log:{loglevel:"debug"},inbounds:[{tag:"socks-in",listen:"127.0.0.1",port:10810,protocol:"socks",settings:{auth:"noauth","udp":true}}],outbounds:[.outbounds[]|del(.streamSettings.sockopt)],routing:{rules:[{type:"field",inboundTag:["socks-in"],outboundTag:$s}]}}' "$CFG" > "$TMP/xray-nomark.json"
jq '(.outbounds|map(select(.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks"))[0].tag) as $s | {log:{loglevel:"debug"},inbounds:[{tag:"socks-in",listen:"127.0.0.1",port:10811,protocol:"socks",settings:{auth:"noauth","udp":true}}],outbounds:[.outbounds[]|if (.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks") then .streamSettings.sockopt=((.streamSettings.sockopt//{})+{mark:255}) else . end],routing:{rules:[{type:"field",inboundTag:["socks-in"],outboundTag:$s}]}}' "$CFG" > "$TMP/xray-mark.json"

run_test() {
	NAME="$1"; CONF="$2"; PORT="$3"
	echo "----------- $NAME (socks 127.0.0.1:$PORT) -----------"
	if [ ! -s "$CONF" ]; then echo "  !! config build failed ($CONF empty)"; return; fi
	# sanity: does this xray accept the built config at all?
	echo "  xray -test:"; "$XRAY" -test -config "$CONF" 2>&1 | tail -3 | sed 's/^/    /'
	# bounded foreground run in background subshell -> reliable full-log capture
	timeout 7 "$XRAY" run -config "$CONF" > "$TMP/$NAME.log" 2>&1 &
	XPID=$!
	sleep 2
	echo "  listening on :$PORT ?"; { ss -ltn 2>/dev/null || netstat -ltn 2>/dev/null; } | grep ":$PORT " | sed 's/^/    /'
	printf "  SOCKS exit-IP (want the SERVER ip): "
	curl -s -x socks5h://127.0.0.1:"$PORT" -m 10 https://api.ipify.org; echo
	curl -s -x socks5h://127.0.0.1:"$PORT" -m 10 -o /dev/null -w '  gstatic=%{http_code}\n' https://www.gstatic.com/generate_204
	sleep 1
	kill "$XPID" 2>/dev/null; wait "$XPID" 2>/dev/null
	echo "  --- FULL xray debug log ---"
	if [ -s "$TMP/$NAME.log" ]; then sed 's/^/    /' "$TMP/$NAME.log"; else echo "    (log empty — xray produced no output)"; fi
	echo
}

echo "=================== decisive standalone test ==================="
run_test xray-nomark "$TMP/xray-nomark.json" 10810
run_test xray-mark   "$TMP/xray-mark.json"   10811

echo "=================== how to read it ============================"
echo "  TLS/DPI probe resets/timeouts  -> DPI cuts reality's SNI/dest (need new SNI/dest or ws transport)"
echo "  xray log 'connection reset'    -> DPI on the tunnel handshake"
echo "  xray log 'i/o timeout'/'dial'  -> path to server blocked mid-handshake"
echo "  xray log 'REALITY'/'invalid'   -> reality params (pbk/sid/sni) mismatch with server"
echo "  both carry IP                  -> outbound fine; bug is the TPROXY LAN capture"
echo "==============================================================="
