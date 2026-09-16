"""Check the application-host guard without running provisioning tasks."""
import pathlib
import subprocess
import tempfile
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]


class ApplicationHostTests(unittest.TestCase):
    def test_refuses_git_host_and_ci_flags(self):
        task = yaml.safe_load((ROOT / "playbooks/prod-server.yml").read_text())[0]["pre_tasks"][0]
        for flag in [None, "forgejo_enabled", "forgejo_runner_enabled", "woodpecker_agent_enabled"]:
            with self.subTest(flag=flag), tempfile.TemporaryDirectory(prefix="pp-host-guard-") as temp:
                play = [{"hosts": "all", "gather_facts": False, "vars": {flag: True} if flag else {}, "tasks": [task]}]
                path = pathlib.Path(temp) / "test.yml"
                path.write_text(yaml.safe_dump(play))
                result = subprocess.run(
                    ["ansible-playbook", "-i", "localhost,", "-c", "local", str(path), "--check"],
                    cwd=ROOT, capture_output=True, text=True,
                )
                self.assertEqual(result.returncode == 0, flag is None, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
