"""Check the application-host guard without running provisioning tasks."""
import pathlib
import subprocess
import tempfile
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]


class ApplicationHostTests(unittest.TestCase):
    def test_refuses_git_host_and_ci_flags(self):
        tasks = yaml.safe_load((ROOT / "playbooks/prod-server.yml").read_text())[0]["pre_tasks"]
        task = next(task for task in tasks if "ansible.builtin.assert" in task)
        for flag in [None, "forgejo_enabled", "forgejo_runner_enabled", "woodpecker_agent_enabled", "coolify_enabled", "reverse_proxy_stop_coolify_proxy"]:
            with self.subTest(flag=flag), tempfile.TemporaryDirectory(prefix="pp-host-guard-") as temp:
                play = [{"hosts": "all", "gather_facts": False, "vars": {flag: True} if flag else {}, "tasks": [task]}]
                path = pathlib.Path(temp) / "test.yml"
                path.write_text(yaml.safe_dump(play))
                result = subprocess.run(
                    ["ansible-playbook", "-i", "localhost,", "-c", "local", str(path), "--check"],
                    cwd=ROOT, capture_output=True, text=True,
                )
                self.assertEqual(result.returncode == 0, flag is None, result.stdout + result.stderr)

    def test_optional_credentials_cannot_bypass_role_validation(self):
        tasks = yaml.safe_load((ROOT / "playbooks/prod-server.yml").read_text())[0]["pre_tasks"]
        for content, valid in [(None, True), ("admin_password: fixture\n", True),
                               ("coolify_enabled: true\n", False), ("[broken", False)]:
            with self.subTest(content=content), tempfile.TemporaryDirectory(prefix="pp-credentials-") as temp:
                root = pathlib.Path(temp)
                (root / "playbooks").mkdir()
                (root / "host_vars").mkdir()
                if content is not None:
                    (root / "host_vars/localhost.credentials.local.yml").write_text(content)
                playbook = root / "playbooks/test.yml"
                playbook.write_text(yaml.safe_dump([{"hosts": "all", "gather_facts": False, "tasks": tasks}]))
                result = subprocess.run(
                    ["ansible-playbook", "-i", "localhost,", "-c", "local", str(playbook), "--check"],
                    cwd=ROOT, capture_output=True, text=True,
                )
                self.assertEqual(result.returncode == 0, valid, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
