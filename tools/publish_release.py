#!/usr/bin/env python3
"""Create-or-update a GitHub release for a tag and upload asset files.

Idempotent: reuses an existing release for the tag, and replaces any asset
whose name already exists. Token comes from the GH_TOKEN env var.
"""
import json
import os
import sys
import urllib.request
import urllib.error

OWNER, REPO = "miroslavrov", "keen-manager"
API = f"https://api.github.com/repos/{OWNER}/{REPO}"
UPLOADS = f"https://uploads.github.com/repos/{OWNER}/{REPO}"
TOKEN = os.environ["GH_TOKEN"]
TAG = sys.argv[1]
TARGET = sys.argv[2]
ASSETS = sys.argv[3:]


def req(url, method="GET", data=None, headers=None, ctype="application/json"):
    h = {
        "Authorization": f"token {TOKEN}",
        "Accept": "application/vnd.github+json",
        "User-Agent": "keen-manager-release",
    }
    if headers:
        h.update(headers)
    if data is not None and ctype == "application/json":
        data = json.dumps(data).encode()
        h["Content-Type"] = "application/json"
    r = urllib.request.Request(url, data=data, method=method, headers=h)
    try:
        with urllib.request.urlopen(r) as resp:
            body = resp.read()
            return resp.status, (json.loads(body) if body else {})
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")


# 1. Get or create the release for TAG.
status, rel = req(f"{API}/releases/tags/{TAG}")
if status == 200:
    print(f"release exists for {TAG} (id={rel['id']})")
else:
    body = {
        "tag_name": TAG,
        "name": f"keen-manager {TAG}",
        "prerelease": True,
        "body": (
            "Unified VPN (Xray / AmneziaWG) + DPI-bypass manager for Keenetic + Entware.\n\n"
            "- fix(route,awg,engine): skip wrong-arch /opt/sbin/ip — fixes routing "
            "\"exec format error\" (falls back to the firmware ip, logs a hint).\n"
            "- feat(web): upload an AmneziaWG .conf by file (picker + drag-and-drop).\n\n"
            "Install / upgrade (Entware):\n\n"
            "```sh\n"
            "opkg update && opkg install curl\n"
            "curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/install.sh | sh\n"
            "```\n\n"
            "Verify: `sha256sum -c sha256sums.txt`"
        ),
    }
    # Only set target_commitish when creating a NEW tag; GitHub 422s if the tag
    # already exists (as here, where we pushed it first).
    if TARGET and TARGET != "-":
        body["target_commitish"] = TARGET
    status, rel = req(f"{API}/releases", "POST", body)
    if status not in (200, 201):
        print("create release FAILED:", status, rel)
        sys.exit(1)
    print(f"created release {TAG} (id={rel['id']})")

rel_id = rel["id"]

# 2. Existing assets, so we can replace by name.
_, assets = req(f"{API}/releases/{rel_id}/assets")
existing = {a["name"]: a["id"] for a in assets} if isinstance(assets, list) else {}

# 3. Upload each file, deleting a prior copy of the same name first.
for path in ASSETS:
    name = os.path.basename(path)
    if name in existing:
        req(f"{API}/releases/assets/{existing[name]}", "DELETE")
        print(f"  deleted old asset {name}")
    with open(path, "rb") as f:
        data = f.read()
    ct = "text/plain" if name.endswith(".txt") else "application/gzip"
    st, res = req(
        f"{UPLOADS}/releases/{rel_id}/assets?name={name}",
        "POST", data, {"Content-Type": ct}, ctype="raw",
    )
    if st in (200, 201):
        print(f"  uploaded {name} ({len(data)} bytes)")
    else:
        print(f"  upload {name} FAILED: {st} {res}")
        sys.exit(1)

print(f"\nRelease ready: https://github.com/{OWNER}/{REPO}/releases/tag/{TAG}")
