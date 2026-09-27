#!/usr/bin/env python3
"""Publish immutable executables and a complete channel release snapshot."""
import hashlib
import json
import pathlib
import re
import shutil
import sys
import subprocess

root = pathlib.Path(sys.argv[1])
revision, sequence, image = sys.argv[2:5]
if not re.fullmatch(r"[a-f0-9]{40}", revision):
    raise SystemExit("invalid revision")
if not re.fullmatch(r"ghcr.io/lesomnus/cxz@sha256:[a-f0-9]{64}", image):
    raise SystemExit("manager image must be pinned by digest")
if int(sequence) <= 0:
    raise SystemExit("invalid CI publication sequence")
tag = sys.argv[5] if len(sys.argv) > 5 else "edge"
if tag != "edge" and not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
    raise SystemExit("channel manifests require edge or a stable tag")
version = "source-" + revision[:12] if tag == "edge" else tag
output = root / "auto-update"
output.mkdir(exist_ok=True)
assets = {}
for system in ("linux", "windows"):
    for arch in ("amd64", "arm64"):
        extension = ".exe" if system == "windows" else ""
        source = root / f"{system}-{arch}" / f"cxz{extension}"
        name = f"cxz-{revision}-{system}-{arch}{extension}"
        target = output / name
        shutil.copyfile(source, target)
        assets[f"{system}/{arch}"] = {
            "name": name, "sha256": hashlib.sha256(target.read_bytes()).hexdigest()
        }
ancestors = subprocess.check_output(["git", "rev-list", "--max-count=4096", revision], text=True).splitlines()[1:]
manifest = dict(tag=tag, version=version, ancestors=ancestors, revision=revision, sequence=int(sequence), protocol=1, schema=2,
                image=image, assets=assets)
(root / "cxz-update.json").write_text(json.dumps(manifest, indent=2) + "\n")
