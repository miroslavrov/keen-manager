#!/bin/sh
# keen-manager — tunnel diagnostic (session 14).
#
# WHY: `xray -test` passes and Xray listens on 127.0.0.1:10808, but
# `curl -x socks5h://127.0.0.1:10808` carries nothing — so the failure is in the
# server OUTBOUND (vless/reality handshake), not in the router routing. This
# script isolates that: it builds TWO standalone SOCKS-only debug Xray configs
# from the live config and runs each in turn:
#   * xray-nomark  (127.0.0.1:10810) — server outbound with SO_MARK removed
#   * xray-mark    (127.0.0.1:10811) — server outbound with SO_MARK 255 (as shipped)
# and probes each through its SOCKS. The debug log shows exactly where the
# handshake dies (reality/TLS reset = DPI, dial timeout = endpoint blocked).
#
# SAFETY: read-only. It does NOT touch the keen-manager service, iptables, ip
# rules or routes. It only spawns short-lived xray processes on 10810/10811 and
# kills them. Run it with the blanc subscription/tunnel OFF (so the router has
# normal internet and :10808 is free); the script uses its own ports regardless.
#
# Usage (one line, no copy-paste pain):
#   curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/diag-tunnel.sh | sh

CFG="${KEEN_XRAY_CONFIG:-/opt/etc/keen-manager/xray/config.json}"
XRAY="${XRAY_BIN:-/opt/sbin/xray}"
TMP="${KEEN_TMP:-/opt/tmp}"
mkdir -p "$TMP"

echo "=================== keen-manager tunnel diag ==================="
[ -x "$XRAY" ] || { echo "!! no xray at $XRAY"; exit 1; }
"$XRAY" -version 2>/dev/null | head -1
[ -f "$CFG" ] || { echo "!! no config at $CFG — activate a server once so it is generated"; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "!! jq missing (opkg install jq). Tell the agent and it will ship a jq-free build."; exit 1; }
echo

# ---- 1) endpoint + direct TCP reachability ---------------------------------
EP=$(jq -r '(.outbounds[]|select(.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks")|(.settings.vnext//.settings.servers)[0]|"\(.address) \(.port)")' "$CFG" 2>/dev/null | head -1)
H=$(echo "$EP" | awk '{print $1}')
P=$(echo "$EP" | awk '{print $2}')
echo "server endpoint : $H:$P"
printf "direct TCP      : "
curl -s --connect-timeout 6 -o /dev/null -w 'time_connect=%{time_connect}s http_code=%{http_code}\n' "https://$H:$P/" 2>&1
echo

# ---- 2) build two SOCKS-only debug configs (no test()/regex; jq-safe) ------
jq '(.outbounds|map(select(.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks"))[0].tag) as $s | {log:{loglevel:"debug"},inbounds:[{tag:"socks-in",listen:"127.0.0.1",port:10810,protocol:"socks",settings:{auth:"noauth",udp:true}}],outbounds:[.outbounds[]|del(.streamSettings.sockopt)],routing:{rules:[{type:"field",inboundTag:["socks-in"],outboundTag:$s}]}}' "$CFG" > "$TMP/xray-nomark.json"
jq '(.outbounds|map(select(.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks"))[0].tag) as $s | {log:{loglevel:"debug"},inbounds:[{tag:"socks-in",listen:"127.0.0.1",port:10811,protocol:"socks",settings:{auth:"noauth",udp:true}}],outbounds:[.outbounds[]|if (.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks") then .streamSettings.sockopt=((.streamSettings.sockopt//{})+{mark:255}) else . end],routing:{rules:[{type:"field",inboundTag:["socks-in"],outboundTag:$s}]}}' "$CFG" > "$TMP/xray-mark.json"

run_test() {
	NAME="$1"; CONF="$2"; PORT="$3"
	echo "----------- $NAME (socks 127.0.0.1:$PORT) -----------"
	if [ ! -s "$CONF" ]; then echo "  !! config build failed ($CONF empty) — jq problem"; return; fi
	"$XRAY" run -c "$CONF" > "$TMP/$NAME.log" 2>&1 &
	PID=$!
	sleep 3
	printf "  SOCKS exit-IP (want the SERVER ip): "
	curl -s -x socks5h://127.0.0.1:"$PORT" -m 15 https://api.ipify.org; echo
	curl -s -x socks5h://127.0.0.1:"$PORT" -m 15 -o /dev/null -w '  gstatic=%{http_code}\n' https://www.gstatic.com/generate_204
	kill "$PID" 2>/dev/null
	sleep 1
	echo "  --- debug log (last 25 lines) ---"
	tail -n 25 "$TMP/$NAME.log" | sed 's/^/  /'
	echo
}

echo "=================== 3) decisive standalone test ==============="
run_test xray-nomark "$TMP/xray-nomark.json" 10810
run_test xray-mark   "$TMP/xray-mark.json"   10811

echo "=================== how to read it ============================"
echo "  nomark works, mark fails  -> SO_MARK 255 collides with routing (fix: outbound.go)"
echo "  both fail                 -> outbound broken: read the debug log —"
echo "                               'REALITY'/TLS/'connection reset' = DPI;"
echo "                               'dial tcp ... i/o timeout' = endpoint blocked"
echo "  both work                 -> outbound fine; bug is the TPROXY LAN capture"
echo "==============================================================="
