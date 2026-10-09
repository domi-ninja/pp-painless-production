#!/usr/bin/env python3
"""Reconcile only UFW rules carrying the pp-managed comment."""
import json
import re
import shlex
import subprocess
import sys


def run(*args):
    return subprocess.run(["ufw", *args], check=True, capture_output=True, text=True).stdout


def desired_rules(config):
    desired = set()
    for protocol in ("tcp", "udp"):
        for value in config[protocol]:
            if not isinstance(value, (str, int)) or isinstance(value, bool) or not re.fullmatch(r"[0-9]+", str(value)):
                raise ValueError("firewall ports must be integers")
            port = int(value)
            if not 1 <= port <= 65535:
                raise ValueError("firewall ports must be numbers between 1 and 65535")
            desired.add(f"{port}/{protocol}")
    return desired


def reconcile(config, preview=False):
    desired = desired_rules(config)
    owned = set()
    for line in run("show", "added").splitlines():
        if "pp-managed" not in line:
            continue
        parts = shlex.split(line)
        if len(parts) != 5 or parts[:2] != ["ufw", "allow"] or parts[3:] != ["comment", "pp-managed"]:
            raise ValueError("unrecognized pp-managed UFW rule; inspect it before retrying")
        owned.add(parts[2])
    if not preview:
        # Open new ports, including the current SSH connection port, before revoking old rules.
        for rule in sorted(desired - owned):
            run("allow", rule, "comment", "pp-managed")
        for rule in sorted(owned - desired):
            run("--force", "delete", "allow", rule)
    print("changed" if desired != owned else "unchanged")


if __name__ == "__main__":
    reconcile(json.load(sys.stdin), preview="--check" in sys.argv[1:])
