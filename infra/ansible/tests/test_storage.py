"""Check Docker relocation against disposable paths and mocked system commands."""
import importlib.util
import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("storage", ROOT / "roles/docker_data_root/files/relocate-docker.py")
STORAGE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(STORAGE)


class StorageTests(unittest.TestCase):
    def test_default_and_custom_roots_copy_the_actual_source(self):
        for source_name in ("var-lib-docker", "custom-root"):
            with self.subTest(source=source_name), tempfile.TemporaryDirectory(prefix="pp-storage-") as temp:
                base = pathlib.Path(temp)
                source, target = base / source_name, base / "new-root"
                source.mkdir()
                config = base / "daemon.json"
                config.write_text(json.dumps({"data-root": str(source), "operator-setting": True}))
                calls = []
                def fake(*args):
                    calls.append(args)
                    if args[:2] == ("docker", "info"):
                        return json.loads(config.read_text())["data-root"]
                    return "container-id" if args == ("docker", "ps", "-q") else ""
                with patch.object(STORAGE, "run", fake):
                    self.assertTrue(STORAGE.apply({"relocate": True, "target": str(target), "backup_confirmed": True,
                                                   "daemon_config": {"live-restore": True}}, config))
                self.assertIn(("rsync", "-aHAX", "--numeric-ids", str(source) + "/", str(target) + "/"), calls)
                self.assertLess(calls.index(("docker", "stop", "--time", "30", "container-id")),
                                calls.index(("systemctl", "stop", "docker.service")))
                self.assertTrue(json.loads(config.read_text())["operator-setting"])
                self.assertTrue(source.exists())

    def test_failed_copy_restores_original_configuration(self):
        with tempfile.TemporaryDirectory(prefix="pp-storage-") as temp:
            base = pathlib.Path(temp)
            source, target = base / "source", base / "target"
            source.mkdir()
            config = base / "daemon.json"
            original = json.dumps({"data-root": str(source)}).encode()
            config.write_bytes(original)
            calls = []
            def fake(*args):
                calls.append(args)
                if args[0] == "rsync":
                    raise RuntimeError("copy failed")
                if args[:2] == ("docker", "info"):
                    return str(source)
                return "container-id" if args == ("docker", "ps", "-q") else ""
            with patch.object(STORAGE, "run", fake), self.assertRaisesRegex(RuntimeError, "previous daemon configuration restored"):
                STORAGE.apply({"relocate": True, "target": str(target), "backup_confirmed": True, "daemon_config": {}}, config)
            self.assertEqual(config.read_bytes(), original)
            self.assertIn(("systemctl", "restart", "docker.service"), calls)
            self.assertIn(("docker", "start", "container-id"), calls)

    def test_overlap_missing_backup_and_nonempty_destination_are_rejected(self):
        with tempfile.TemporaryDirectory(prefix="pp-storage-") as temp:
            source = pathlib.Path(temp) / "source"
            source.mkdir()
            target = pathlib.Path(temp) / "target"
            target.mkdir()
            (target / "old-data").write_text("preserve")
            for destination, confirmed in [(source / "child", True), (source.parent, True),
                                           (target, True), (pathlib.Path(temp) / "empty", False)]:
                with self.subTest(destination=destination), self.assertRaises(ValueError):
                    STORAGE.validate_paths(str(source), str(destination), confirmed)


if __name__ == "__main__":
    unittest.main()
