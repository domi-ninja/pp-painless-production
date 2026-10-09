#!/usr/bin/env python3
"""Apply Docker settings and, when requested, copy its stopped data store."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout.strip()


def validate_paths(current, target, confirmed):
    source, destination = Path(current), Path(target)
    for path in (source, destination):
        if not path.is_absolute() or str(path) == "/" or path.resolve() != path:
            raise ValueError("Docker roots must be canonical absolute paths without symlinks")
    if not source.is_dir():
        raise ValueError("current Docker root is not a directory")
    if source == destination:
        return False
    if not confirmed:
        raise ValueError("confirm a verified backup before relocating Docker data")
    if source in destination.parents or destination in source.parents:
        raise ValueError("Docker source and destination must not overlap")
    if destination.exists() and (not destination.is_dir() or any(destination.iterdir())):
        raise ValueError("Docker destination must be empty; inspect any previous copy first")
    return True


def merge(original, desired):
    result = dict(original)
    for key, value in desired.items():
        result[key] = merge(result.get(key, {}), value) if isinstance(value, dict) and isinstance(result.get(key, {}), dict) else value
    return result


def atomic_write(path, body):
    descriptor, name = tempfile.mkstemp(prefix=".pp-docker-", dir=path.parent)
    try:
        with os.fdopen(descriptor, "wb") as file:
            file.write(body)
            file.flush()
            os.fsync(file.fileno())
        os.chmod(name, 0o644)
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def apply(settings, config_path=Path("/etc/docker/daemon.json")):
    if config_path.is_symlink():
        raise ValueError("Docker daemon configuration must not be a symlink")
    if "data-root" in settings["daemon_config"]:
        raise ValueError("set docker_data_root_path, not data-root inside docker_daemon_config")
    original = config_path.read_bytes() if config_path.exists() else None
    old_config = json.loads(original) if original else {}
    current = run("docker", "info", "--format", "{{.DockerRootDir}}")
    target = settings["target"] if settings["relocate"] else current
    moving = validate_paths(current, target, settings["backup_confirmed"])
    desired = merge(old_config, settings["daemon_config"])
    desired["data-root"] = target
    if not moving and desired == old_config:
        return False
    body = (json.dumps(desired, indent=2, sort_keys=True) + "\n").encode()
    config_path.parent.mkdir(parents=True, exist_ok=True)
    # The source is preserved. This backup also survives a killed provisioning client.
    if original is not None:
        atomic_write(config_path.with_suffix(".json.pp-before"), original)
    running = run("docker", "ps", "-q").splitlines() if moving else []
    try:
        if moving:
            if running:
                run("docker", "stop", "--time", "30", *running)
            run("systemctl", "stop", "docker.socket")
            run("systemctl", "stop", "docker.service")
            Path(target).mkdir(parents=True, exist_ok=True)
            run("rsync", "-aHAX", "--numeric-ids", current + "/", target + "/")
        atomic_write(config_path, body)
        run("systemctl", "start", "docker.socket")
        run("systemctl", "restart", "docker.service")
        if run("docker", "info", "--format", "{{.DockerRootDir}}") != target:
            raise RuntimeError("Docker did not start with the requested data root")
        if running:
            run("docker", "start", *running)
    except Exception as failure:
        try:
            if original is None:
                config_path.unlink(missing_ok=True)
            else:
                atomic_write(config_path, original)
            run("systemctl", "start", "docker.socket")
            run("systemctl", "restart", "docker.service")
            if run("docker", "info", "--format", "{{.DockerRootDir}}") != current:
                raise RuntimeError("restored Docker root does not match the original")
            if running:
                run("docker", "start", *running)
        except Exception as recovery:
            raise RuntimeError("Docker change failed and restoring its previous configuration also failed; inspect Docker before retrying") from recovery
        raise RuntimeError("Docker change failed; previous daemon configuration restored and original data preserved") from failure
    return True


if __name__ == "__main__":
    try:
        print("changed" if apply(json.load(sys.stdin)) else "unchanged")
    except Exception as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
