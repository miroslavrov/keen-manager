#!/bin/sh
# keen-manager installer (curl | sh).
#
#   opkg update && opkg install curl
#   curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/install.sh | sh
#
# Detects the router architecture, downloads the matching static binary from the
# GitHub release, installs the init script + ndm netfilter hook, and starts the
# service. Idempotent: safe to re-run to upgrade in place. On download failure
# any existing install is left untouched.
#
# Environment overrides:
#   REPO          GitHub owner/repo            (default miroslavrov/keen-manager)
#   KEEN_VERSION  release tag to install       (default: latest)
#   KEEN_URL      full URL to the .gz binary   (overrides REPO/KEEN_VERSION/ARCH)
#   KEEN_ARCH     force arch (mipsle|mips|arm64|arm), skip detection
#   KEEN_PORT     web UI port to print         (default 8088)

set -e

# --- Constants --------------------------------------------------------------
REPO="${REPO:-miroslavrov/keen-manager}"
KEEN_VERSION="${KEEN_VERSION:-latest}"
KEEN_PORT="${KEEN_PORT:-8088}"

BIN_DIR="/opt/bin"
BIN_PATH="${BIN_DIR}/keen-manager"
INITD_DIR="/opt/etc/init.d"
INIT_PATH="${INITD_DIR}/S99keen-manager"

# --- Output helpers ---------------------------------------------------------
log()  { echo "[keen-manager] $*"; }
warn() { echo "[keen-manager] WARNING: $*" >&2; }
err()  { echo "[keen-manager] ERROR: $*" >&2; }
die()  { err "$*"; exit 1; }

# --- Preflight: Entware present ---------------------------------------------
if [ ! -d "$BIN_DIR" ]; then
	die "Entware not found (missing ${BIN_DIR}). Install Entware/opkg on this router first."
fi

# --- Preflight: download tool ----------------------------------------------
# Prefer curl; fall back to wget if that is what the box has.
DL=""
if command -v curl >/dev/null 2>&1; then
	DL="curl"
elif command -v wget >/dev/null 2>&1; then
	DL="wget"
else
	die "neither curl nor wget found. Run: opkg update && opkg install curl"
fi

# download <url> <dest> — writes to <dest>, returns non-zero on failure.
download() {
	_url="$1"; _dest="$2"
	if [ "$DL" = "curl" ]; then
		curl -fsSL "$_url" -o "$_dest"
	else
		wget -q -O "$_dest" "$_url"
	fi
}

# --- Arch detection (mirrors internal/platform/arch.go) ---------------------
# Prefer `opkg print-architecture`, then uname -m + ELF endianness probe.
detect_arch_opkg() {
	command -v opkg >/dev/null 2>&1 || return 1
	# Lines look like: "arch mipselsf 100". Skip all/noarch.
	opkg print-architecture 2>/dev/null | while read -r _kw _name _prio; do
		[ "$_kw" = "arch" ] || continue
		case "$_name" in
			all|noarch) continue ;;
			aarch64*)              echo arm64;  return 0 ;;
			mipselsf*|mipsel*)     echo mipsle; return 0 ;;
			mipssf*|mips*)         echo mips;   return 0 ;;
			armv7*|arm*)           echo arm;    return 0 ;;
		esac
	done
	# The subshell above prints on match; capture handled by caller via command
	# substitution. Nothing printed => unknown.
}

# elf_is_big_endian: inspect EI_DATA (byte offset 5) of a system binary.
# 0x01 = little-endian, 0x02 = big-endian. Uses od (busybox has it).
elf_is_big_endian() {
	for _p in /bin/sh /bin/busybox /opt/bin/sh; do
		[ -r "$_p" ] || continue
		# Read the 6th byte (offset 5). od -An -tx1 -j5 -N1 prints e.g. " 02".
		_b=$(od -An -tx1 -j5 -N1 "$_p" 2>/dev/null | tr -d ' \n')
		[ -n "$_b" ] || continue
		[ "$_b" = "02" ] && return 0
		return 1
	done
	return 1
}

detect_arch_uname() {
	_m=$(uname -m 2>/dev/null)
	case "$_m" in
		aarch64|arm64) echo arm64 ;;
		armv7*|arm*)   echo arm ;;
		mips|mips64)
			if elf_is_big_endian; then
				echo mips
			else
				echo mipsle
			fi
			;;
		*) echo "" ;;
	esac
}

resolve_arch() {
	if [ -n "$KEEN_ARCH" ]; then
		echo "$KEEN_ARCH"
		return 0
	fi
	_a=$(detect_arch_opkg 2>/dev/null | head -n1)
	if [ -z "$_a" ]; then
		_a=$(detect_arch_uname)
	fi
	echo "$_a"
}

ARCH=$(resolve_arch)
case "$ARCH" in
	mipsle|mips|arm64|arm) ;;
	*)
		die "could not determine a supported architecture (got: '${ARCH:-unknown}'). Supported: mipsle, mips, arm64, arm. Override with KEEN_ARCH=..."
		;;
esac
log "detected architecture: ${ARCH}"

# --- Resolve download URL ---------------------------------------------------
if [ -n "$KEEN_URL" ]; then
	URL="$KEEN_URL"
elif [ "$KEEN_VERSION" = "latest" ]; then
	URL="https://github.com/${REPO}/releases/latest/download/keen-manager-${ARCH}.gz"
else
	URL="https://github.com/${REPO}/releases/download/${KEEN_VERSION}/keen-manager-${ARCH}.gz"
fi
log "downloading ${URL}"

# --- Download to a temp file, verify, then install atomically ---------------
TMP_GZ=$(mktemp /tmp/keen-manager.XXXXXX.gz 2>/dev/null || echo /tmp/keen-manager.$$.gz)
TMP_BIN="${TMP_GZ%.gz}.bin"
cleanup() { rm -f "$TMP_GZ" "$TMP_BIN"; }
trap cleanup EXIT INT TERM

if ! download "$URL" "$TMP_GZ"; then
	die "download failed from ${URL}. Existing install (if any) left untouched."
fi
if [ ! -s "$TMP_GZ" ]; then
	die "downloaded file is empty. Existing install (if any) left untouched."
fi

# Decompress. gunzip -t first so a bad body doesn't clobber a working binary.
if ! gunzip -t "$TMP_GZ" 2>/dev/null; then
	die "downloaded file is not a valid gzip archive. Existing install left untouched."
fi
gunzip -c "$TMP_GZ" > "$TMP_BIN"
if [ ! -s "$TMP_BIN" ]; then
	die "decompressed binary is empty. Existing install left untouched."
fi
chmod +x "$TMP_BIN"

# Install atomically: mv over the old binary only after we have a good one.
mkdir -p "$BIN_DIR"
mv -f "$TMP_BIN" "$BIN_PATH"
chmod +x "$BIN_PATH"
log "installed binary -> ${BIN_PATH}"

# Best-effort: show the version we just put down.
if "$BIN_PATH" version >/dev/null 2>&1; then
	log "version: $("$BIN_PATH" version 2>/dev/null | head -n1)"
fi

# --- Install the init script (inline; no network dependency) ----------------
mkdir -p "$INITD_DIR"
cat > "$INIT_PATH" <<'INIT_EOF'
#!/bin/sh
# S99keen-manager — Entware init script for the keen-manager daemon.
#
# Runs `keen-manager daemon` in the background, tracks it with a PID file, and
# appends output to a log file. Installed by the keen-manager installer.
#
# Usage: S99keen-manager {start|stop|restart|status|check}

NAME="keen-manager"
BIN="/opt/bin/keen-manager"
ARGS="daemon"
RUN_DIR="/opt/var/run"
PIDFILE="${RUN_DIR}/keen-manager.pid"
LOG_DIR="/opt/var/log"
LOGFILE="${LOG_DIR}/keen-manager.log"

is_running() {
	[ -f "$PIDFILE" ] || return 1
	_pid=$(cat "$PIDFILE" 2>/dev/null)
	[ -n "$_pid" ] || return 1
	if kill -0 "$_pid" 2>/dev/null; then
		return 0
	fi
	return 1
}

running_pid() {
	cat "$PIDFILE" 2>/dev/null
}

start() {
	if is_running; then
		echo "$NAME already running (pid $(running_pid))"
		return 0
	fi
	if [ ! -x "$BIN" ]; then
		echo "$NAME: binary not found or not executable at $BIN" >&2
		echo "$NAME: (re)install with the keen-manager installer" >&2
		return 1
	fi
	mkdir -p "$RUN_DIR" "$LOG_DIR" 2>/dev/null
	echo "Starting $NAME ..."
	if command -v setsid >/dev/null 2>&1; then
		setsid "$BIN" $ARGS >>"$LOGFILE" 2>&1 &
	else
		"$BIN" $ARGS >>"$LOGFILE" 2>&1 &
	fi
	echo $! > "$PIDFILE"
	sleep 1
	if is_running; then
		echo "$NAME started (pid $(running_pid)); logging to $LOGFILE"
		return 0
	fi
	echo "$NAME failed to start; see $LOGFILE" >&2
	rm -f "$PIDFILE"
	return 1
}

stop() {
	if ! is_running; then
		echo "$NAME not running"
		rm -f "$PIDFILE"
		return 0
	fi
	_pid=$(running_pid)
	echo "Stopping $NAME (pid $_pid) ..."
	kill "$_pid" 2>/dev/null
	_i=0
	while [ "$_i" -lt 10 ]; do
		if ! kill -0 "$_pid" 2>/dev/null; then
			break
		fi
		sleep 1
		_i=$((_i + 1))
	done
	if kill -0 "$_pid" 2>/dev/null; then
		echo "$NAME did not stop gracefully; sending SIGKILL"
		kill -9 "$_pid" 2>/dev/null
	fi
	rm -f "$PIDFILE"
	echo "$NAME stopped"
	return 0
}

restart() {
	stop
	sleep 1
	start
}

status() {
	if is_running; then
		echo "$NAME is running (pid $(running_pid))"
		return 0
	fi
	echo "$NAME is stopped"
	return 1
}

check() {
	if is_running; then
		return 0
	fi
	echo "$NAME not running; starting"
	start
}

case "$1" in
	start)   start ;;
	stop)    stop ;;
	restart) restart ;;
	status)  status ;;
	check)   check ;;
	*)
		echo "Usage: $0 {start|stop|restart|status|check}" >&2
		exit 1
		;;
esac

exit $?
INIT_EOF
chmod +x "$INIT_PATH"
log "installed init script -> ${INIT_PATH}"

# --- Install the ndm netfilter hook (best-effort) ---------------------------
# The binary owns the hook contents; we just ask it to write them. A failure
# here is non-fatal — TPROXY/kill-switch is opt-in and off by default.
if "$BIN_PATH" install-hook >/dev/null 2>&1; then
	log "installed ndm netfilter hook (route reapply)"
else
	warn "could not install ndm netfilter hook via 'keen-manager install-hook'."
	warn "transparent-proxy rules will not auto-reapply after a topology change until you run it manually."
fi

# --- (Re)start the service --------------------------------------------------
log "starting service ..."
if [ -x "$INIT_PATH" ]; then
	# On upgrade the daemon may already be running; restart to pick up the new binary.
	if "$INIT_PATH" status >/dev/null 2>&1; then
		"$INIT_PATH" restart || warn "service restart reported an error; check /opt/var/log/keen-manager.log"
	else
		"$INIT_PATH" start || warn "service start reported an error; check /opt/var/log/keen-manager.log"
	fi
fi

# --- Report the web UI URL --------------------------------------------------
# Best-effort LAN IP discovery: Keenetic's LAN bridge is usually br0.
lan_ip() {
	# 1) ip addr show br0
	if command -v ip >/dev/null 2>&1; then
		_ip=$(ip -4 addr show br0 2>/dev/null | grep -o 'inet [0-9.]*' | awk '{print $2}' | head -n1)
		[ -n "$_ip" ] && { echo "$_ip"; return 0; }
		# any non-loopback v4 address as a second try
		_ip=$(ip -4 addr show 2>/dev/null | grep -o 'inet [0-9.]*' | awk '{print $2}' | grep -v '^127\.' | head -n1)
		[ -n "$_ip" ] && { echo "$_ip"; return 0; }
	fi
	# 2) hostname -i
	_ip=$(hostname -i 2>/dev/null | awk '{print $1}')
	case "$_ip" in
		""|127.*) ;;
		*) echo "$_ip"; return 0 ;;
	esac
	# 3) fallback
	echo "192.168.1.1"
}

IP=$(lan_ip)
log "done."
log "Web UI: http://${IP}:${KEEN_PORT}"
log "Manage the service with: ${INIT_PATH} {start|stop|restart|status}"
