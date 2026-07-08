#!/bin/sh
# keen-manager uninstaller.
#
#   sh /opt/etc/init.d/../../.. # (or run from the repo)
#   curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/uninstall.sh | sh
#
# Stops the service, removes the init script, the ndm netfilter hook, and the
# binary. User configuration and state under /opt/etc/keen-manager are KEPT by
# default; pass --purge to remove them too.
#
# This runs without prompting, but prints every action it takes.

set -e

INIT_PATH="/opt/etc/init.d/S99keen-manager"
BIN_PATH="/opt/bin/keen-manager"
NDM_DIR="/opt/etc/ndm/netfilter.d"
DATA_DIR="/opt/etc/keen-manager"
PIDFILE="/opt/var/run/keen-manager.pid"

PURGE=0
for arg in "$@"; do
	case "$arg" in
		--purge) PURGE=1 ;;
		-h|--help)
			echo "Usage: uninstall.sh [--purge]"
			echo "  --purge   also remove ${DATA_DIR} (config, state, secrets, backups)"
			exit 0
			;;
		*) echo "[keen-manager] unknown option: $arg" >&2; exit 1 ;;
	esac
done

log() { echo "[keen-manager] $*"; }

# --- Stop the service -------------------------------------------------------
if [ -x "$INIT_PATH" ]; then
	log "stopping service ..."
	"$INIT_PATH" stop 2>/dev/null || log "service was not running (or stop failed); continuing"
else
	# Init script gone but a daemon may still be running from a stale PID file.
	if [ -f "$PIDFILE" ]; then
		_pid=$(cat "$PIDFILE" 2>/dev/null)
		if [ -n "$_pid" ] && kill -0 "$_pid" 2>/dev/null; then
			log "killing stray daemon (pid $_pid) ..."
			kill "$_pid" 2>/dev/null || true
		fi
	fi
fi
rm -f "$PIDFILE"

# --- Remove the init script -------------------------------------------------
if [ -e "$INIT_PATH" ]; then
	log "removing init script ${INIT_PATH}"
	rm -f "$INIT_PATH"
fi

# --- Remove the ndm netfilter hook(s) ---------------------------------------
# The binary names the hook; match anything mentioning keen-manager to be safe.
if [ -d "$NDM_DIR" ]; then
	_found=0
	for f in "$NDM_DIR"/*keen-manager* "$NDM_DIR"/*keen_manager*; do
		[ -e "$f" ] || continue
		log "removing ndm hook ${f}"
		rm -f "$f"
		_found=1
	done
	[ "$_found" -eq 0 ] && log "no ndm netfilter hook found (nothing to remove)"
fi

# --- Remove the binary ------------------------------------------------------
if [ -e "$BIN_PATH" ]; then
	log "removing binary ${BIN_PATH}"
	rm -f "$BIN_PATH"
fi

# --- Config / state ---------------------------------------------------------
if [ "$PURGE" -eq 1 ]; then
	if [ -d "$DATA_DIR" ]; then
		log "purging config/state ${DATA_DIR}"
		rm -rf "$DATA_DIR"
	fi
else
	if [ -d "$DATA_DIR" ]; then
		log "keeping config/state ${DATA_DIR} (pass --purge to remove)"
	fi
fi

log "uninstall complete."
