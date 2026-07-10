#!/bin/sh
# keen-manager — tunnel diagnostic v4 (session 17).
#
# WHY v4 (session-16 pivot): the SAME vless-reality-vision config connects from
# the user's PC and phone on the SAME LAN — only the router fails. Verified vs
# the XKeen canon, our outbound is byte-for-byte correct, so the bug is NOT the
# config's shape. The one thing that differs between the (working) phone and the
# (failing) router is HOW traffic enters Xray: the router uses a transparent
# TPROXY + dokodemo-door capture, while the phone uses a plain SOCKS/tun client.
# v3 was inconclusive because it hid xray's own output behind `| tail` and never
# proved xray even started. v4 answers three questions UNAMBIGUOUSLY:
#
#   A) FREEDOM control (socks -> freedom)     : does xray RUN on this device and
#      is the box's own egress alive?  A pass returns the ROUTER's WAN IP.
#   B) REALITY via plain SOCKS (socks -> srv) : does the active server carry
#      traffic the SAME way the phone reaches it — NO tproxy/dokodemo? A pass
#      returns the SERVER's IP.
#   C) it NEVER hides failure: it echoes the generated config, prints the full
#      `xray -test` output + its exit code, says whether the process stayed
#      alive, and cats the whole run log (never `| tail`).
#
# READ THE RESULT (this is the session-17 P0.2 fork):
#   * A empty  -> xray does NOT start here, or the box has no direct internet.
#                 Nothing else below is meaningful. Read the -test output + log.
#   * A passes, B passes (SERVER ip) -> reality works through plain SOCKS, just
#                 like the phone. The bug is the ROUTER's TPROXY/dokodemo capture
#                 -> switch the install to the Proxy-client path (Settings ->
#                 Xray integration = "proxy"). This is the likely outcome.
#   * A passes, B empty -> reality from THIS box fails even through plain SOCKS
#                 (deeper than capture: server/transport/DPI on this node). Read
#                 the log line (reset / i/o timeout / REALITY invalid) and retry
#                 with a WS node from the subscription instead of reality-vision.
#
# SAFETY: read-only. Does NOT touch the keen-manager service, iptables, ip rules,
# routes, or the live config. Spawns a short-lived xray on 10814/10815 with a
# self-terminating watchdog (busybox has no `timeout`). Safe to run with the
# tunnel ON or OFF — it uses its own ports and its own throwaway configs.
#
# Reality CANNOT be validated from the cloud sandbox (it MITMs TLS); this script
# is meant to run ON THE DEVICE:
#   curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/<commit>/scripts/diag-tunnel.sh | sh

CFG="${KEEN_XRAY_CONFIG:-/opt/etc/keen-manager/xray/config.json}"
XRAY="${XRAY_BIN:-/opt/sbin/xray}"
TMP="${KEEN_TMP:-/opt/tmp}"
FPORT="${KEEN_FREEDOM_PORT:-10814}"   # socks -> freedom control
RPORT="${KEEN_REALITY_PORT:-10815}"   # socks -> server (reality) probe
WATCHDOG="${KEEN_WATCHDOG_SECS:-45}"  # backstop kill if the script is interrupted
mkdir -p "$TMP" 2>/dev/null

# redact_secrets masks the credential fields (vless/vmess "id", trojan/ss
# "password") when we echo a generated config, so a pasted diag never leaks the
# live UUID/password. Uses busybox-safe BRE only (no oniguruma).
redact_secrets() {
	sed -e 's/\("id"[[:space:]]*:[[:space:]]*"\)[^"]*"/\1***redacted***"/g' \
	    -e 's/\("password"[[:space:]]*:[[:space:]]*"\)[^"]*"/\1***redacted***"/g'
}

echo "=================== keen-manager tunnel diag v4 ==================="
echo "date        : $(date 2>/dev/null)"
echo "xray bin    : $XRAY"
if [ ! -x "$XRAY" ]; then echo "!! FATAL: no executable xray at $XRAY"; exit 1; fi
VER=$("$XRAY" -version 2>&1 | head -1)
echo "xray version: ${VER:-<none — xray -version printed nothing>}"
echo "config      : $CFG"
if [ ! -f "$CFG" ]; then
	echo "!! FATAL: no config at $CFG — activate a server once so it is generated."
	exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
	echo "!! FATAL: jq missing (opkg install jq) — needed to read the live config."
	exit 1
fi
echo

# ---- what is the active server? (from the live config) ---------------------
# jq here uses ONLY == / or (busybox jq has no oniguruma → no test()/match()).
SRVFILTER='.outbounds[]|select(.protocol=="vless" or .protocol=="vmess" or .protocol=="trojan" or .protocol=="shadowsocks")'
EP=$(jq -r "[$SRVFILTER]|.[0]|((.settings.vnext//.settings.servers)[0]|\"\(.address) \(.port)\")" "$CFG" 2>/dev/null | head -1)
H=$(echo "$EP" | awk '{print $1}'); P=$(echo "$EP" | awk '{print $2}')
NET=$(jq -r "[$SRVFILTER]|.[0]|(.streamSettings.network//\"tcp\")" "$CFG" 2>/dev/null | head -1)
SEC=$(jq -r "[$SRVFILTER]|.[0]|(.streamSettings.security//\"none\")" "$CFG" 2>/dev/null | head -1)
FLOW=$(jq -r "[$SRVFILTER]|.[0]|((.settings.vnext[0].users[0].flow)//\"\")" "$CFG" 2>/dev/null | head -1)
SNI=$(jq -r "[$SRVFILTER]|.[0]|(.streamSettings.realitySettings.serverName//.streamSettings.tlsSettings.serverName//\"\")" "$CFG" 2>/dev/null | head -1)
echo "=== active server (from live config) ==="
echo "  endpoint  : ${H:-?}:${P:-?}"
echo "  transport : network=${NET:-?} security=${SEC:-?} flow=${FLOW:-<none>}"
echo "  reality SNI: ${SNI:-<none>}"
if [ -z "$H" ] || [ "$H" = "null" ]; then
	echo "  !! could not read a server outbound from the config — cannot run probe B."
fi
printf "  direct TCP to server : "
curl -s --connect-timeout 6 -o /dev/null -w 'connect=%{time_connect}s http=%{http_code}\n' "https://$H:$P/" 2>&1
echo

# ---- WAN interface + MTU (context only) ------------------------------------
echo "=== WAN / MTU context ==="
WANDEV=$(ip route show default 2>/dev/null | sed -n 's/.* dev \([^ ]*\).*/\1/p' | head -1)
echo "  default route dev : ${WANDEV:-?}"
[ -n "$WANDEV" ] && ip link show "$WANDEV" 2>/dev/null | grep -o 'mtu [0-9]*' | sed 's/^/  WAN /'
printf "  router direct WAN IP: "
curl -s -m 8 "https://api.ipify.org" 2>/dev/null; echo "  (this is what probe A should return)"
echo

# ---- config builders (throwaway; live config is never touched) -------------
# Freedom control: 100% static, no jq — proves xray starts and egress is alive
# regardless of the server. If THIS is empty, stop reading: xray isn't running.
write_freedom() {   # write_freedom OUTFILE PORT
	cat > "$1" <<EOF
{
  "log": { "loglevel": "warning" },
  "inbounds": [ { "tag": "socks-in", "listen": "127.0.0.1", "port": $2, "protocol": "socks", "settings": { "auth": "noauth", "udp": true } } ],
  "outbounds": [ { "tag": "direct", "protocol": "freedom" } ],
  "routing": { "rules": [ { "type": "field", "inboundTag": [ "socks-in" ], "outboundTag": "direct" } ] }
}
EOF
}

# Reality-via-SOCKS: keep the live server outbound VERBATIM (incl. sockopt mark
# 255) and just point a fresh SOCKS inbound at it — this is the phone's path
# (plain client, NO tproxy/dokodemo). If B passes but the router doesn't, the
# capture is the culprit.
write_reality() {   # write_reality OUTFILE PORT
	jq "([$SRVFILTER][0]) as \$srv
	   | { log: { loglevel: \"warning\" },
	       inbounds: [ { tag: \"socks-in\", listen: \"127.0.0.1\", port: $2, protocol: \"socks\", settings: { auth: \"noauth\", udp: true } } ],
	       outbounds: [ \$srv, { tag: \"direct\", protocol: \"freedom\" } ],
	       routing: { rules: [ { type: \"field\", inboundTag: [ \"socks-in\" ], outboundTag: \$srv.tag } ] } }" \
	  "$CFG" > "$1" 2>"$1.jqerr"
}

# run_variant NAME CONFIG PORT — prints EVERYTHING, hides NOTHING.
run_variant() {
	NAME="$1"; CONF="$2"; PORT="$3"
	echo "----------------- $NAME (socks 127.0.0.1:$PORT) -----------------"

	if [ ! -s "$CONF" ]; then
		echo "  !! generated config is EMPTY — build failed. jq error:"
		[ -f "$CONF.jqerr" ] && sed 's/^/     /' "$CONF.jqerr"
		echo
		return
	fi
	echo "  --- generated config ($CONF; id/password redacted) ---"
	redact_secrets < "$CONF" | sed 's/^/    /'

	echo "  --- xray -test ---"
	"$XRAY" -test -config "$CONF" > "$TMP/$NAME.test" 2>&1
	RC=$?
	sed 's/^/    /' "$TMP/$NAME.test"
	echo "    xray -test exit code: $RC"
	if [ "$RC" != "0" ]; then
		echo "  !! config REJECTED by xray -test (exit $RC) — not starting it."
		echo
		return
	fi

	echo "  --- launching xray run (auto-stops after the probes; ${WATCHDOG}s watchdog) ---"
	"$XRAY" run -config "$CONF" > "$TMP/$NAME.log" 2>&1 &
	XPID=$!
	( sleep "$WATCHDOG"; kill "$XPID" 2>/dev/null ) &
	WPID=$!
	sleep 2

	if kill -0 "$XPID" 2>/dev/null; then
		echo "    process: ALIVE (pid $XPID)"
	else
		echo "    process: EXITED EARLY — xray quit within 2s (see run log below)."
	fi
	printf "    listening on :%s ? " "$PORT"
	{ ss -ltn 2>/dev/null || netstat -ltn 2>/dev/null; } | grep ":$PORT " >/dev/null 2>&1 \
		&& echo "yes" || echo "NO"

	printf "    SOCKS exit IP : "
	curl -s -x "socks5h://127.0.0.1:$PORT" -m 8 "https://api.ipify.org" 2>&1; echo
	curl -s -x "socks5h://127.0.0.1:$PORT" -m 8 -o /dev/null \
		-w '    generate_204  : http=%{http_code} time=%{time_total}s\n' \
		"https://www.gstatic.com/generate_204" 2>&1
	# a larger body flushes out MTU-only failures a tiny GET can mask
	curl -s -x "socks5h://127.0.0.1:$PORT" -m 10 -o /dev/null \
		-w '    1MB fetch     : http=%{http_code} bytes=%{size_download} time=%{time_total}s\n' \
		"https://speed.cloudflare.com/__down?bytes=1048576" 2>&1

	kill "$XPID" 2>/dev/null; wait "$XPID" 2>/dev/null; kill "$WPID" 2>/dev/null

	echo "    --- full run log ($TMP/$NAME.log) ---"
	if [ -s "$TMP/$NAME.log" ]; then
		sed 's/^/      /' "$TMP/$NAME.log"
	else
		echo "      (run log EMPTY — xray printed nothing; combine with the process/listening lines above)"
	fi
	echo
}

write_freedom "$TMP/diag-freedom.json" "$FPORT"
write_reality "$TMP/diag-reality.json" "$RPORT"

echo "=================== A) FREEDOM control ==================="
echo "  PASS = returns the ROUTER's own WAN IP (proves xray runs + egress alive)."
run_variant freedom "$TMP/diag-freedom.json" "$FPORT"

echo "=================== B) REALITY via plain SOCKS ==================="
echo "  PASS = returns the SERVER's IP (the phone's path — NO tproxy/dokodemo)."
run_variant reality "$TMP/diag-reality.json" "$RPORT"

echo "=================== how to read it (session-17 P0.2 fork) ==================="
echo "  A empty                 -> xray isn't starting here / no direct internet. Read A's -test + log."
echo "  A ok  + B = SERVER ip   -> reality works via plain SOCKS like the phone => the TPROXY/dokodemo"
echo "                             capture is the bug. Set Settings -> Xray integration = \"proxy\"."
echo "  A ok  + B empty         -> reality fails from this box even via SOCKS (server/transport/DPI on"
echo "                             this node). Read B's log (reset / i/o timeout / REALITY invalid),"
echo "                             then retry with a WS node instead of reality-vision."
echo "==========================================================================="
