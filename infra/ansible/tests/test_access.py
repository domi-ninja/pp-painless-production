"""Exercise access validation and firewall changes without provisioning a host."""
import importlib.util
import pathlib
import subprocess
import tempfile
import unittest
import copy
from unittest.mock import patch

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("firewall", ROOT / "roles/base/files/reconcile-firewall.py")
FIREWALL = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(FIREWALL)


class AccessTests(unittest.TestCase):
    def test_key_validation_rejects_placeholders_and_invalid_blobs(self):
        tasks = yaml.safe_load((ROOT / "roles/base/tasks/validate-access.yml").read_text())
        with tempfile.TemporaryDirectory(prefix="pp-access-") as temp:
            key = pathlib.Path(temp) / "key"
            subprocess.run(["ssh-keygen", "-t", "ed25519", "-N", "", "-f", str(key)], check=True, capture_output=True)
            for public_key, valid in [(key.with_suffix(".pub").read_text().strip(), True),
                                      ("ssh-ed25519 REPLACE_WITH_YOUR_PUBLIC_KEY", False),
                                      ("ssh-ed25519 AAAA invalid", False),
                                      ("ssh-ed25519 AAAA\nssh-ed25519 AAAA", False)]:
                with self.subTest(key=public_key):
                    play = [{"hosts": "all", "gather_facts": False,
                             "vars": {"ssh_port": 22, "ssh_password_authentication": False,
                                      "admin_ssh_public_keys": [public_key]}, "tasks": tasks}]
                    path = pathlib.Path(temp) / "play.yml"
                    path.write_text(yaml.safe_dump(play))
                    result = subprocess.run(["ansible-playbook", "-i", "localhost,", "-c", "local", str(path), "--check"],
                                            cwd=ROOT, capture_output=True, text=True)
                    self.assertEqual(result.returncode == 0, valid, result.stdout + result.stderr)

    def test_effective_ssh_guard_catches_earlier_overrides(self):
        tasks = yaml.safe_load((ROOT / "roles/base/tasks/configure-ssh.yml").read_text())
        block = tasks[2]["block"]
        with tempfile.TemporaryDirectory(prefix="pp-sshd-") as temp:
            config = pathlib.Path(temp) / "sshd_config"
            managed = pathlib.Path(temp) / "00-pp.conf"
            managed.write_text("Port 22\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nPermitRootLogin no\n")
            for prefix, valid in [("", True), ("PasswordAuthentication yes\n", False)]:
                with self.subTest(prefix=prefix):
                    config.write_text(prefix + "Include " + str(managed) + "\n")
                    checks = copy.deepcopy(block[3:])
                    checks[0]["ansible.builtin.command"]["argv"][1:1] = ["-f", str(config)]
                    play = [{"hosts": "all", "gather_facts": False,
                             "vars": {"admin_user": "deploy", "ssh_port": 22,
                                      "ssh_password_authentication": False, "ssh_permit_root_login": "no"},
                             "tasks": checks}]
                    path = pathlib.Path(temp) / "play.yml"
                    path.write_text(yaml.safe_dump(play))
                    result = subprocess.run(["ansible-playbook", "-i", "localhost,", "-c", "local", str(path)],
                                            cwd=ROOT, capture_output=True, text=True)
                    self.assertEqual(result.returncode == 0, valid, result.stdout + result.stderr)

    def test_firewall_opens_new_ports_before_revoking_owned_rules(self):
        calls = []
        def fake(*args):
            calls.append(args)
            return "ufw allow 22/tcp comment 'pp-managed'\nufw allow 8080/tcp comment 'pp-managed'\nufw allow 9999/tcp\n" if args == ("show", "added") else ""
        with patch.object(FIREWALL, "run", fake):
            FIREWALL.reconcile({"tcp": [22, 443], "udp": []})
        self.assertEqual(calls[1:], [("allow", "443/tcp", "comment", "pp-managed"),
                                    ("--force", "delete", "allow", "8080/tcp")])

    def test_firewall_preview_and_invalid_ports_do_not_change_rules(self):
        with patch.object(FIREWALL, "run", return_value="") as run:
            FIREWALL.reconcile({"tcp": [22], "udp": []}, preview=True)
            self.assertEqual(run.call_count, 1)
            with self.assertRaises(ValueError):
                FIREWALL.reconcile({"tcp": [70000], "udp": []})
            self.assertEqual(run.call_count, 1)


if __name__ == "__main__":
    unittest.main()
