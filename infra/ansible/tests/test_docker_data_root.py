"""Exercise the data-root guard without stopping Docker or copying its state."""
import pathlib
import subprocess
import tempfile
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]


class DockerDataRootTests(unittest.TestCase):
    def test_rejects_overlapping_paths_before_mutation(self):
        tasks = yaml.safe_load((ROOT / "roles/docker_data_root/tasks/main.yml").read_text())
        first_mutation = next(i for i, task in enumerate(tasks)
                              if task["name"] == "Ensure Docker data root directory exists")
        guard = [task for task in tasks[:first_mutation]
                 if task["name"] not in ["Check Docker CLI is available", "Read current Docker data root"]]
        with tempfile.TemporaryDirectory(prefix="pp-docker-root-") as temp:
            root = pathlib.Path(temp)
            source = root / "docker"
            source.mkdir()
            alias = root / "alias"
            alias.symlink_to(source, target_is_directory=True)
            cases = [
                ("unchanged", source, source, True),
                ("sibling", source, root / "docker-archive", True),
                ("destination inside source", source, source / "child", False),
                ("destination contains source", source, root, False),
                ("normalized descendant", source, source / "unused/../child", False),
                ("normalized same root", source, source / "unused/..", True),
                ("symlink descendant", source, alias / "child", False),
                ("symlink source ancestor", alias, root, False),
                ("symlink same root", alias, source, True),
                ("normalized filesystem root", source, pathlib.Path("/tmp/.."), False),
            ]
            for name, current, destination, allowed in cases:
                with self.subTest(name=name):
                    play = [{"hosts": "all", "gather_facts": False,
                             "vars": {"docker_current_root": {"stdout": str(current)},
                                      "docker_data_root_path": str(destination)},
                             "tasks": guard}]
                    play_path = root / "guard.yml"
                    play_path.write_text(yaml.safe_dump(play))
                    result = subprocess.run(
                        ["ansible-playbook", "-i", "localhost,", "-c", "local", str(play_path)],
                        cwd=ROOT, capture_output=True, text=True,
                    )
                    self.assertEqual(result.returncode == 0, allowed, result.stdout + result.stderr)
                    if not allowed:
                        self.assertIn("Docker data roots must", result.stdout + result.stderr)
                    self.assertFalse((source / "child").exists())


    def test_rejects_bind_mount_layouts_in_private_namespace(self):
        probe = subprocess.run(["unshare", "--mount", "true"], capture_output=True)
        if probe.returncode != 0:
            self.skipTest("requires private mount namespaces")
        tasks = yaml.safe_load((ROOT / "roles/docker_data_root/tasks/main.yml").read_text())
        first_mutation = next(i for i, task in enumerate(tasks)
                              if task["name"] == "Ensure Docker data root directory exists")
        guard = [task for task in tasks[:first_mutation]
                 if task["name"] not in ["Check Docker CLI is available", "Read current Docker data root"]]
        with tempfile.TemporaryDirectory(prefix="pp-docker-bind-") as temp:
            root = pathlib.Path(temp)
            source, alias = root / "source", root / "alias"
            source.mkdir()
            (source / "child").mkdir()
            alias.mkdir()
            cases = [
                ("source bind alias", source, alias, source / "missing-child", False),
                ("target bind alias", source, source / "child", alias, False),
                ("unsupported disjoint bind", source, alias, root / "outside", False),
                ("whole filesystem alias", pathlib.Path("/"), source,
                 alias / source.relative_to("/") / "missing-child", False),
                ("same bind root no-op", source, alias, alias, True),
            ]
            for name, mount_source, current, destination, allowed in cases:
                with self.subTest(name=name):
                    play = [{"hosts": "all", "gather_facts": False,
                             "vars": {"docker_current_root": {"stdout": str(current)},
                                      "docker_data_root_path": str(destination)},
                             "tasks": guard}]
                    play_path = root / "guard.yml"
                    play_path.write_text(yaml.safe_dump(play))
                    command = ["unshare", "--mount", "sh", "-c",
                               'set -eu; mount --make-rprivate /; mount --bind "$1" "$2"; shift 2; exec "$@"',
                               "sh", str(mount_source), str(alias), "ansible-playbook", "-i", "localhost,",
                               "-c", "local", str(play_path)]
                    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
                    self.assertEqual(result.returncode == 0, allowed, result.stdout + result.stderr)
                    if not allowed:
                        self.assertIn("direct, unaliased filesystem mounts", result.stdout + result.stderr)
                    self.assertFalse((source / "missing-child").exists())

if __name__ == "__main__":
    unittest.main()
