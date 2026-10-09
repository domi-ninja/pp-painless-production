"""Run a private SeaweedFS store with only its authenticated S3 port exposed."""
import json
import os
import re
import subprocess
import sys
import tempfile

BUCKET_KEYS = [f"S3_STORAGE_{name}_BUCKET" for name in
               ["EXPORTS", "SNAPSHOT_IMPORTS", "MODULES", "FILES", "SEARCH"]]


def configuration(values):
    buckets = list(dict.fromkeys(values.get(key, "") for key in BUCKET_KEYS))
    if any(not re.fullmatch(r"[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]", name) for name in buckets):
        raise ValueError("Invalid storage bucket name")
    access, secret = values.get("STORAGE_ACCESS_KEY"), values.get("STORAGE_SECRET_KEY")
    if not access or not secret:
        raise ValueError("Missing storage credentials")
    identity = {"name": "convex", "credentials": [{"accessKey": access, "secretKey": secret}],
                "actions": [f"{action}:{bucket}" for bucket in buckets for action in ["Read", "Write", "List"]]}
    return {"identities": [identity]}, buckets


def client(values):
    import boto3
    from botocore.config import Config
    return boto3.client("s3", endpoint_url="http://127.0.0.1:9000", region_name="us-east-1",
                        aws_access_key_id=values["STORAGE_ACCESS_KEY"],
                        aws_secret_access_key=values["STORAGE_SECRET_KEY"],
                        config=Config(connect_timeout=2, read_timeout=3,
                                      retries={"max_attempts": 0}, s3={"addressing_style": "path"}))


def main():
    config, buckets = configuration(os.environ)
    if sys.argv[1:] == ["health"]:
        s3 = client(os.environ)
        for bucket in buckets:
            s3.head_bucket(Bucket=bucket)
        return
    if sys.argv[1:]:
        raise ValueError("Use health or no arguments")
    os.umask(0o077)
    with tempfile.NamedTemporaryFile(mode="w", prefix="pp-storage-", suffix=".json", delete=False) as file:
        json.dump(config, file)
        config_path = file.name
    # mini shares one bind address. Forward only S3; internal APIs stay on loopback.
    subprocess.Popen(["socat", "TCP-LISTEN:9000,bind=0.0.0.0,reuseaddr,fork", "TCP:127.0.0.1:8333"])
    os.execvp("weed", ["weed", "mini", "-dir=/data", "-ip=127.0.0.1", "-ip.bind=127.0.0.1",
                       "-s3.config=" + config_path, "-bucket=" + ",".join(buckets),
                       "-webdav=false", "-admin.ui=false", "-master.telemetry=false",
                       "-s3.port.iceberg=0", "-s3.port.lance=0", "-s3.autoCreateBucket=false",
                       "-s3.allowDeleteBucketNotEmpty=false"])


if __name__ == "__main__":
    try:
        main()
    except Exception:
        print("Storage startup or readiness check failed", file=sys.stderr)
        sys.exit(1)
