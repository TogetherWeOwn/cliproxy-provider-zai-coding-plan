#!/usr/bin/env bash
# Local release packaging only. Does not upload assets or modify a host service.
set -euo pipefail

if [[ $# != 1 || "$1" != /* ]]; then
  echo 'Usage: scripts/package-release.sh /absolute/output-directory' >&2
  exit 2
fi
root=$(dirname "$(dirname "$(realpath "$0")")")
out=$1
cache=${XDG_CACHE_HOME:-"$HOME/.cache"}
mkdir -p "$cache" "$out"
stage=$(mktemp -d "$cache/zai-release.XXXXXX")
trap 'rm -rf -- "$stage"' EXIT

# Version/ID are sourced from the store registry, not from a mutable tag name.
read -r id version < <(python3 -I - "$root/registry.json" <<'PY'
import json, re, sys
p = json.load(open(sys.argv[1], encoding="utf-8"))["plugins"][0]
assert p["id"] == "zai-coding-plan"
assert re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", p["version"])
print(p["id"], p["version"])
PY
)

# Explicitly clear build flags/tags so a hostcompat-tagged test image cannot be
# packaged accidentally. A native Linux amd64 C compiler is required.
if [[ $(go env GOHOSTOS) != linux || $(go env GOHOSTARCH) != amd64 ]]; then
  echo 'Release packaging requires a Linux amd64 builder.' >&2
  exit 2
fi
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 GOFLAGS= go -C "$root" build \
  -trimpath -tags='' -buildmode=c-shared -o "$stage/$id.so" .

python3 -I - "$stage/$id.so" "$out" "$id" "$version" <<'PY'
import hashlib, pathlib, sys, zipfile
lib, out, plugin_id, version = sys.argv[1:]
archive = pathlib.Path(out) / f"{plugin_id}_{version}_linux_amd64.zip"
with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as z:
    # Stable member metadata; the compiler determines binary reproducibility.
    info = zipfile.ZipInfo(f"{plugin_id}.so", date_time=(1980, 1, 1, 0, 0, 0))
    info.external_attr = 0o100644 << 16
    info.compress_type = zipfile.ZIP_DEFLATED
    z.writestr(info, pathlib.Path(lib).read_bytes())
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
(pathlib.Path(out) / "checksums.txt").write_text(f"{digest}  {archive.name}\n", encoding="ascii")
print(archive.name)
print("checksums.txt")
PY
