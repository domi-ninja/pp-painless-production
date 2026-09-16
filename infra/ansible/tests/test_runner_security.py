"""Run only the guard assertions locally; never provision a host in these tests."""
import pathlib
import subprocess
import tempfile
import unittest

import jinja2
import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]


class RunnerSecurityTests(unittest.TestCase):
    def assert_guard(self, task, groups, variables, allowed):
        with tempfile.TemporaryDirectory(prefix="pp-runner-guard-") as temp:
            base = pathlib.Path(temp)
            inventory = {group: {"hosts": {"localhost": {"ansible_connection": "local"}}} for group in groups}
            (base / "inventory.yml").write_text(yaml.safe_dump(inventory))
            play = [{"hosts": "all", "gather_facts": False, "vars": variables, "tasks": [task]}]
            (base / "test.yml").write_text(yaml.safe_dump(play))
            result = subprocess.run(
                ["ansible-playbook", "-i", str(base / "inventory.yml"), str(base / "test.yml"), "--check"],
                cwd=ROOT, capture_output=True, text=True,
            )
            self.assertEqual(result.returncode == 0, allowed, result.stdout + result.stderr)

    def test_production_refuses_either_runner(self):
        task = yaml.safe_load((ROOT / "playbooks/prod-server.yml").read_text())[0]["pre_tasks"][0]
        self.assert_guard(task, ["prod_servers"], {}, True)
        for setting in ["forgejo_runner_enabled", "woodpecker_agent_enabled"]:
            self.assert_guard(task, ["prod_servers"], {setting: True}, False)

    def test_roles_require_dedicated_group_and_explicit_isolation(self):
        task = yaml.safe_load((ROOT / "roles/runner_guard/tasks/main.yml").read_text())[0]
        self.assert_guard(task, ["runner_servers"], {}, False)
        self.assert_guard(task, ["runner_servers", "prod_servers"], {"ci_runner_isolated_host": True}, False)
        self.assert_guard(task, ["runner_servers"], {"ci_runner_isolated_host": True}, True)

    def test_application_containers_block_runner_install(self):
        task = yaml.safe_load((ROOT / "roles/runner_guard/tasks/main.yml").read_text())[2]
        self.assert_guard(task, ["runner_servers"], {"runner_pp_containers": {"stdout": "existing-container"}}, False)

    def test_dind_has_no_tcp_listener(self):
        template = (ROOT / "roles/forgejo_runner/templates/compose.yml.j2").read_text()
        defaults = yaml.safe_load((ROOT / "roles/forgejo_runner/defaults/main.yml").read_text())
        environment = jinja2.Environment(undefined=jinja2.StrictUndefined)
        environment.filters["bool"] = bool
        compose = yaml.safe_load(environment.from_string(template).render(defaults))
        dind = compose["services"]["docker-in-docker"]
        runner = compose["services"]["runner"]
        self.assertFalse(any("tcp://" in arg for arg in dind["command"]))
        self.assertNotIn("ports", dind)
        self.assertEqual(runner["environment"]["DOCKER_HOST"], "unix:///run/runner-docker/docker.sock")
        self.assertIn("runner-docker-socket:/run/runner-docker", dind["volumes"])
        self.assertIn("runner-docker-socket:/run/runner-docker", runner["volumes"])


if __name__ == "__main__":
    unittest.main()
