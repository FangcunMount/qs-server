"""Freeze original API transaction identities in the owned production profile."""

import json
import os
from pathlib import Path
import re
import sys

START = "# BEGIN FINAL API MESSAGING\n"
END = "# END FINAL API MESSAGING\n"
SUB_START = "# BEGIN FINAL IAM SUBSCRIPTION\n"
SUB_END = "# END FINAL IAM SUBSCRIPTION\n"


def production_binding(environment):
    if environment.get("MESSAGING_PROVIDER") != "rocketmq":
        raise ValueError("Explicit final RocketMQ provider required")
    endpoint = environment.get("ROCKETMQ_ENDPOINT", "")
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9.-]*:[0-9]{1,5}", endpoint) or not 1 <= int(endpoint.rsplit(":", 1)[1]) <= 65535:
        raise ValueError("Explicit endpoint required")
    if environment.get("ROCKETMQ_INSTANCE_ID") != "qs-apiserver-1":
        raise ValueError("Original API subscription identity required")
    admission = environment.get("ROCKETMQ_PUBLISH_ENABLED", "false")
    if admission not in {"true", "false"}:
        raise ValueError("Explicit admission required")
    root = "/app/configs/rocketmq/"
    owners = {}
    for store in ("mysql", "mongo"):
        identity = {}
        for field in ("store_id", "writer_epoch"):
            value = environment.get("ROCKETMQ_" + store.upper() + "_" + field.upper(), "")
            if not value.strip() or not 1 <= len(value.encode("utf-8")) <= 128 or "\x00" in value:
                raise ValueError("Original installed transaction identity required")
            identity[field] = value  # Preserve bytes, including case, whitespace and dollars.
        identity.update(
            credentials_file=root + ("credentials.json" if store == "mysql" else "mongo-credentials.json"),
            route_revision="m3-1",
            limits={"sends": 4, "branches": 16, "check_workers": 2, "check_queue": 16,
                    "reconcile_batch": 50, "check_timeout": "2s", "rpc_timeout": "5s",
                    "lease": "60s", "reconcile_interval": "5s"},
        )
        owners["qs." + store] = identity
    return {"enabled": True, "provider": "rocketmq", "rocketmq": {
        "endpoint": endpoint, "credentials_file": root + "credentials.json",
        "ca_file": root + "ca.crt", "routes_file": root + "routes.json",
        "instance_id": "qs-apiserver-1", "publish_enabled": admission == "true",
        "transactions": owners,
    }}


def main():
    try:
        binding = production_binding(os.environ)
        if sys.argv[1:] == ["validate"]:
            return
        if len(sys.argv) != 3 or sys.argv[1] != "render":
            raise ValueError("Expected validate or render CONFIG")
        path = Path(sys.argv[2])
        text = path.read_text()
        transport = {k: v for k, v in binding["rocketmq"].items() if k != "transactions"}
        transport.update(transactions={}, publish_enabled=False)
        subscription = {"enabled": True, "provider": "rocketmq", "topic": "iam.authz.version.v2",
                        "channel-prefix": "qs-authz-sync", "ephemeral-nsq": False, "rocketmq": transport}
        for start, end, content in (
            (START, END, "messaging: " + json.dumps(binding, ensure_ascii=True)),
            (SUB_START, SUB_END, "  authz-sync: " + json.dumps(subscription, ensure_ascii=True)),
        ):
            if text.count(start) != 1 or text.count(end) != 1:
                raise ValueError("Owned production template required")
            before, body = text.split(start)
            _, after = body.split(end)
            text = before + start + content + "\n" + end + after
        path.write_text(text)
    except (ValueError, TypeError, OSError):
        raise SystemExit("Invalid final API messaging configuration; contents withheld") from None


if __name__ == "__main__":
    main()
