#!/usr/bin/env python3
"""Remove obsolete SHA-named edge executables only after manifest publication."""
import json
import os
import re
import subprocess
import sys

ASSET = re.compile(r"cxz-([a-f0-9]{40})-(linux-(?:amd64|arm64)|windows-(?:amd64|arm64)\.exe)")
PLATFORMS = ("linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64")


def obsolete_assets(manifest, assets):
    revision = manifest.get("revision", "")
    if manifest.get("tag") != "edge" or not re.fullmatch(r"[a-f0-9]{40}", revision):
        raise ValueError("cleanup requires an edge manifest with a full revision")
    keep = set()
    for platform in PLATFORMS:
        extension = ".exe" if platform.startswith("windows/") else ""
        expected = f"cxz-{revision}-{platform.replace('/', '-')}{extension}"
        if manifest.get("assets", {}).get(platform, {}).get("name") != expected:
            raise ValueError("manifest has incomplete or invalid platform assets")
        keep.add(expected)
    uploaded = {a["name"] for a in assets if a.get("state") == "uploaded" and a.get("size", 0) > 0}
    if not keep <= uploaded:
        raise ValueError("current publication is incomplete; refusing cleanup")
    return [a for a in assets if ASSET.fullmatch(a["name"]) and a["name"] not in keep]


def main():
    repo = os.environ["GITHUB_REPOSITORY"]
    expected = json.loads(open(sys.argv[1], encoding="utf-8").read())

    def gh(*args):
        return subprocess.check_output(["gh", *args], text=True)

    published = json.loads(gh("release", "download", "edge", "--repo", repo,
                              "--pattern", "cxz-update.json", "--output", "-"))
    if published != expected:
        raise ValueError("edge manifest changed or publication failed; refusing cleanup")
    release = json.loads(gh("api", f"repos/{repo}/releases/tags/edge"))
    pages = json.loads(gh("api", "--paginate", "--slurp",
                         f"repos/{repo}/releases/{release['id']}/assets?per_page=100"))
    for asset in obsolete_assets(published, [a for page in pages for a in page]):
        print(f"Deleting obsolete edge asset: {asset['name']}", flush=True)
        gh("api", "--method", "DELETE", f"repos/{repo}/releases/assets/{asset['id']}")


if __name__ == "__main__":
    main()
