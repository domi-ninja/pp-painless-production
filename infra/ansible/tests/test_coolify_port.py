"""Resolve the readiness port from an isolated Coolify environment file."""
import pathlib
import subprocess
import tempfile
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]


class CoolifyPortTests(unittest.TestCase):
    def test_reads_preserved_config_and_compose_default(self):
        tasks = yaml.safe_load((ROOT / "roles/coolify/tasks/main.yml").read_text())
        task = next(task for task in tasks if task["name"] == "Read Coolify application bind port")
        for content, expected in [("", "8000"), ("APP_PORT=\n", "8000"),
                                  ("APP_PORT=9001\n", "9001"), ("APP_PORT='9002'\n", "9002")]:
            with self.subTest(content=content), tempfile.TemporaryDirectory(prefix="pp-coolify-port-") as temp:
                path = pathlib.Path(temp)
                (path / ".env").write_text("APP_KEY=fixture-not-to-be-printed\n" + content)
                play = [{"hosts": "all", "gather_facts": False,
                         "vars": {"coolify_source_dir": temp, "coolify_app_port": ""},
                         "tasks": [task, {"ansible.builtin.assert": {
                             "that": ["coolify_runtime_port.stdout == '" + expected + "'"]}}]}]
                (path / "test.yml").write_text(yaml.safe_dump(play))
                result = subprocess.run(["ansible-playbook", "-i", "localhost,", "-c", "local", str(path / "test.yml")],
                                        cwd=ROOT, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertNotIn("fixture-not-to-be-printed", result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
