"""Deploys frontend/dist to Firebase Hosting via the REST API.

firebase-tools 401s with the service-account key on this machine; the plain
REST flow with a gcloud user token works. Flow: create version (with the
rewrite config from firebase.json) -> populateFiles -> upload gzipped blobs
-> finalize -> release.
"""

import gzip
import hashlib
import json
import os
import subprocess
import sys
import urllib.request

SITE = "crucible-hack-0725"
DIST = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "dist")
API = "https://firebasehosting.googleapis.com/v1beta1"

TOKEN = subprocess.check_output(
    ["gcloud", "auth", "print-access-token"], text=True, shell=True
).strip()

HDRS = {
    "Authorization": f"Bearer {TOKEN}",
    "Content-Type": "application/json",
    "x-goog-user-project": "crucible-hack-0725",
}


def call(url, method="GET", body=None, raw=None, headers=None):
    data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
    req = urllib.request.Request(url, data=data, method=method, headers=headers or HDRS)
    with urllib.request.urlopen(req) as resp:
        text = resp.read()
        return json.loads(text) if text else {}


# Mirrors frontend/firebase.json. The SPA fallback glob "**" must come last —
# Hosting evaluates rewrites in order.
CONFIG = {
    "rewrites": [
        {"glob": "/v1/**", "run": {"serviceId": "crucible-backend", "region": "us-central1"}},
        {"glob": "/health", "run": {"serviceId": "crucible-backend", "region": "us-central1"}},
        {"glob": "**", "path": "/index.html"},
    ],
    "headers": [
        {"glob": "/worklets/**", "headers": {"Cache-Control": "no-cache"}},
    ],
}

# 1. Create a version carrying the config.
version = call(f"{API}/sites/{SITE}/versions", "POST", {"config": CONFIG})
vname = version["name"]
print("version:", vname)

# 2. Hash every dist file (SHA256 of the gzipped bytes).
files = {}
blobs = {}
for root, _, names in os.walk(DIST):
    for name in names:
        full = os.path.join(root, name)
        rel = "/" + os.path.relpath(full, DIST).replace("\\", "/")
        gz = gzip.compress(open(full, "rb").read(), 9)
        digest = hashlib.sha256(gz).hexdigest()
        files[rel] = digest
        blobs[digest] = gz
print(f"{len(files)} files")

# 3. Which blobs does Hosting not already have?
pop = call(f"{API}/{vname}:populateFiles", "POST", {"files": files})
required = pop.get("uploadRequiredHashes", [])
upload_url = pop["uploadUrl"]
print(f"{len(required)} to upload")

# 4. Upload them.
for digest in required:
    call(
        f"{upload_url}/{digest}",
        "POST",
        raw=blobs[digest],
        headers={**HDRS, "Content-Type": "application/octet-stream"},
    )
print("uploaded")

# 5. Finalize and release.
call(f"{API}/{vname}?update_mask=status", "PATCH", {"status": "FINALIZED"})
release = call(f"{API}/sites/{SITE}/releases?versionName={vname}", "POST", {})
print("released:", release.get("name", "?"))
print(f"live at: https://{SITE}.web.app")
