"""Read Docker's resolved Coolify port instead of reparsing Compose dotenv values."""
import json
import os
import pathlib
import subprocess
import tempfile
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]


class CoolifyPortTests(unittest.TestCase):
    def test_reads_resolved_container_port(self):
        tasks = yaml.safe_load((ROOT / "roles/coolify/tasks/main.yml").read_text())
        task = next(task for task in tasks if task["name"] == "Read Coolify application bind port")
        for content, expected in [("", "8000"), ("APP_PORT=\n", "8000"),
                                  ("APP_PORT=9001 # UI port\n", "9001"),
                                  ("APP_PORT='9002' # preserved port\n", "9002")]:
            with self.subTest(content=content), tempfile.TemporaryDirectory(prefix="pp-coolify-port-") as temp:
                path = pathlib.Path(temp)
                (path / ".env").write_text("APP_KEY=fixture-not-to-be-printed\n" + content)
                expected_args = ["inspect", "--format",
                                 '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort }}', "coolify"]
                (path / "docker").write_text(
                    "#!/usr/bin/env python3\nimport sys\n"
                    + "assert sys.argv[1:] == " + repr(expected_args) + "\n"
                    + "print(" + json.dumps(expected) + ")\n")
                (path / "docker").chmod(0o700)
                play = [{"hosts": "all", "gather_facts": False,
                         "environment": {"PATH": temp + os.pathsep + os.environ["PATH"]},
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
