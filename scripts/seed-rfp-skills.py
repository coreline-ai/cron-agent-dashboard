#!/usr/bin/env python3
"""Seed RFP collaboration skills into cron-agent-dashboard.

Registers docs/skills/rfp-skills/payloads.json and assigns skills to the
matching agent names in the workspace. This script never executes skill scripts.
"""

from __future__ import annotations

import argparse
import json
import os
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PAYLOAD = ROOT / "docs" / "skills" / "rfp-skills" / "payloads.json"


def request(method: str, base_url: str, path: str, payload: dict[str, Any] | None = None, token: str = "") -> dict[str, Any]:
    headers = {"Accept": "application/json"}
    data = None
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(base_url.rstrip("/") + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            body = resp.read().decode("utf-8")
            return json.loads(body) if body else {}
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"HTTP {exc.code} {method} {path}: {body}") from exc
    except urllib.error.URLError as exc:
        raise SystemExit(f"Request failed {method} {path}: {exc}") from exc


def main() -> int:
    parser = argparse.ArgumentParser(description="Seed RFP agent skills")
    parser.add_argument("--base-url", default=os.environ.get("DASHBOARD_API", "http://127.0.0.1:8080/api"))
    parser.add_argument("--workspace", default="")
    parser.add_argument("--payload", default=str(DEFAULT_PAYLOAD))
    parser.add_argument("--token", default=os.environ.get("CRON_AGENT_DASHBOARD_TOKEN", os.environ.get("DASHBOARD_TOKEN", "")))
    parser.add_argument("--register-only", action="store_true")
    args = parser.parse_args()

    payload = json.loads(Path(args.payload).read_text())
    workspace = args.workspace or payload.get("workspace_slug")
    if not workspace:
        raise SystemExit("workspace is required")

    skill_ids: dict[str, str] = {}
    for skill in payload.get("skills", []):
        result = request("POST", args.base_url, f"/workspaces/{workspace}/skills", dict(skill), token=args.token)
        saved = result.get("skill") or {}
        skill_ids[saved.get("name") or skill["name"]] = saved["id"]
        print(f"registered: {saved.get('name')} ({saved.get('id')})")

    if args.register_only:
        return 0

    agents = request("GET", args.base_url, f"/workspaces/{workspace}/agents", token=args.token).get("agents") or []
    agents_by_name = {a["name"].lower(): a for a in agents}
    for agent_name, assignments in (payload.get("agent_assignments") or {}).items():
        agent = agents_by_name.get(agent_name.lower())
        if not agent:
            raise SystemExit(f"agent not found: {agent_name}")
        for assignment in assignments:
            skill_name = assignment["skill_name"]
            skill_id = skill_ids.get(skill_name)
            if not skill_id:
                raise SystemExit(f"skill not registered: {skill_name}")
            body = {
                "skill_id": skill_id,
                "activation_mode": assignment.get("activation_mode", "trigger"),
                "priority": assignment.get("priority", 100),
                "enabled": assignment.get("enabled", True),
            }
            request("POST", args.base_url, f"/agents/{agent['id']}/skills", body, token=args.token)
            print(f"assigned: {agent_name} <- {skill_name} ({body['activation_mode']}, p{body['priority']})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
