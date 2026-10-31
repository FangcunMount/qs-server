#!/usr/bin/env python3
"""Reject retired transport/execution code in the actual host dependency graph."""

import sys
from pathlib import Path


def check(path: Path) -> None:
    dependencies = path.read_text().splitlines()
    if not dependencies:
        raise SystemExit("production dependency graph is empty")
    forbidden = {
        "github.com/nsqio/go-nsq",
        "github.com/FangcunMount/reliable-messaging/transport/nsq",
        "github.com/FangcunMount/reliable-messaging/relay",
    }
    found = sorted(
        dep for dep in dependencies
        if dep in forbidden or dep.startswith("github.com/FangcunMount/qs-server/internal/testutil/")
    )
    if found:
        raise SystemExit("retired/test-only messaging dependency: " + ", ".join(found))
    print("QS production dependency graph contains no NSQ, publishing Relay or test fixtures")


if __name__ == "__main__":
    check(Path(sys.argv[1]))
