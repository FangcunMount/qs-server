#!/usr/bin/env python3
"""Enroll testees checked in after a date into a plan through the REST API."""

from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Enroll testees created after a date into an assessment plan."
    )
    parser.add_argument("--base-url", required=True, help="API base URL, e.g. http://127.0.0.1:8080/api/v1")
    parser.add_argument("--token", required=True, help="Bearer token with qs:evaluation_plan_manager or qs:admin")
    parser.add_argument("--plan-id", required=True, help="Target assessment plan ID")
    parser.add_argument("--created-start-date", required=True, help="Check-in start date, YYYY-MM-DD")
    parser.add_argument("--created-end-date", help="Optional inclusive check-in end date, YYYY-MM-DD")
    parser.add_argument(
        "--start-date",
        help="Plan start date for all testees, YYYY-MM-DD. Defaults to --created-start-date.",
    )
    parser.add_argument(
        "--start-date-source",
        choices=("fixed", "created_at"),
        default="fixed",
        help="Use a fixed start date or each testee's created_at date.",
    )
    parser.add_argument("--page-size", type=int, default=100, help="Page size for listing testees")
    parser.add_argument("--sleep-ms", type=int, default=0, help="Delay between enroll calls")
    parser.add_argument("--dry-run", action="store_true", help="Only list matched testees")
    return parser.parse_args()


class APIClient:
    def __init__(self, base_url: str, token: str) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token

    def request(self, method: str, path: str, payload: dict | None = None, query: dict | None = None) -> dict:
        url = self.base_url + path
        if query:
            url += "?" + urllib.parse.urlencode({k: v for k, v in query.items() if v not in (None, "")})

        body = None
        headers = {"Authorization": f"Bearer {self.token}"}
        if payload is not None:
            body = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json"

        req = urllib.request.Request(url, data=body, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as err:
            detail = err.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"{method} {url} failed: HTTP {err.code}: {detail}") from err

    def list_testees(self, page: int, page_size: int, created_start_date: str, created_end_date: str | None) -> dict:
        return self.request(
            "GET",
            "/testees",
            query={
                "created_start_date": created_start_date,
                "created_end_date": created_end_date,
                "page": page,
                "page_size": page_size,
            },
        )

    def enroll_testee(self, plan_id: str, testee_id: str, start_date: str) -> dict:
        return self.request(
            "POST",
            "/plans/enroll",
            payload={"plan_id": plan_id, "testee_id": testee_id, "start_date": start_date},
        )


def response_data(resp: dict) -> dict:
    if resp.get("code") != 0:
        raise RuntimeError(f"API returned non-success response: {resp}")
    data = resp.get("data")
    if not isinstance(data, dict):
        raise RuntimeError(f"API returned invalid data payload: {resp}")
    return data


def resolve_start_date(args: argparse.Namespace, item: dict) -> str:
    if args.start_date_source == "fixed":
        return args.start_date or args.created_start_date

    created_at = str(item.get("created_at") or "")
    if len(created_at) < 10:
        raise RuntimeError(f"testee {item.get('id')} has no usable created_at: {created_at!r}")
    return created_at[:10]


def main() -> int:
    args = parse_args()
    client = APIClient(args.base_url, args.token)

    matched = 0
    enrolled = 0
    failed = 0
    page = 1

    while True:
        data = response_data(
            client.list_testees(page, args.page_size, args.created_start_date, args.created_end_date)
        )
        items = data.get("items") or []
        if not items:
            break

        for item in items:
            testee_id = str(item["id"])
            start_date = resolve_start_date(args, item)
            matched += 1
            if args.dry_run:
                print(f"DRY-RUN testee_id={testee_id} created_at={item.get('created_at', '')} start_date={start_date}")
                continue

            try:
                result = response_data(client.enroll_testee(args.plan_id, testee_id, start_date))
                task_count = len(result.get("tasks") or [])
                enrolled += 1
                print(f"OK testee_id={testee_id} start_date={start_date} tasks={task_count}")
            except Exception as err:
                failed += 1
                print(f"FAIL testee_id={testee_id} error={err}", file=sys.stderr)

            if args.sleep_ms > 0:
                time.sleep(args.sleep_ms / 1000)

        total_pages = int(data.get("total_pages") or page)
        if page >= total_pages:
            break
        page += 1

    print(f"summary matched={matched} enrolled={enrolled} failed={failed} dry_run={args.dry_run}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
