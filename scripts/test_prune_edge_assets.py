import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location("prune", pathlib.Path(__file__).with_name("prune-edge-assets.py"))
prune = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prune)


class PruneTests(unittest.TestCase):
    def publication(self, revision):
        manifest = {"tag": "edge", "revision": revision, "assets": {}}
        assets = []
        for i, platform in enumerate(prune.PLATFORMS):
            suffix = ".exe" if platform.startswith("windows/") else ""
            name = f"cxz-{revision}-{platform.replace('/', '-')}{suffix}"
            manifest["assets"][platform] = {"name": name}
            assets.append({"id": i, "name": name, "state": "uploaded", "size": 42})
        return manifest, assets

    def test_only_obsolete_executables_are_removed(self):
        current, assets = self.publication("a" * 40)
        _, old = self.publication("b" * 40)
        other = [{"name": n} for n in ["cxz-edge-linux-amd64.tar.gz", "SHA256SUMS", "cxz-update.json", "custom-file", "cxz-v0.1.0-linux-amd64.tar.gz"]]
        self.assertEqual(prune.obsolete_assets(current, assets + old + other), old)
        self.assertEqual(prune.obsolete_assets(current, assets + other), [])

    def test_no_cleanup_for_incomplete_publication(self):
        current, assets = self.publication("a" * 40)
        with self.assertRaises(ValueError):
            prune.obsolete_assets(current, assets[:-1])
        assets[0]["state"] = "starter"
        with self.assertRaises(ValueError):
            prune.obsolete_assets(current, assets)

    def test_invalid_or_stable_manifest_is_rejected(self):
        current, assets = self.publication("a" * 40)
        current["tag"] = "v0.1.0"
        with self.assertRaises(ValueError):
            prune.obsolete_assets(current, assets)
        current["tag"] = "edge"
        current["assets"]["linux/amd64"]["name"] = "wrong"
        with self.assertRaises(ValueError):
            prune.obsolete_assets(current, assets)
