#!/usr/bin/env python3
"""Register recommended project skills in a local cron-agent-dashboard server.

The script is intentionally small and dependency-free. It registers skills from
`docs/skills/project-skills/payloads.json` and optionally assigns them to an
agent by id or name. It never executes skill scripts.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PAYLOAD = ROOT / "docs" / "skills" / "project-skills" / "payloads.json"


def request(method: str, base_url: str, path: str, payload: dict[str, Any] | None = None, token: str = "") -> dict[str, Any]:
    data = None
    headers = {"Accept": "application/json"}
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


def resolve_agent_id(base_url: str, workspace: str, agent_id: str, agent_name: str, token: str) -> str:
    if agent_id:
        return agent_id
    if not agent_name:
        return ""
    data = request("GET", base_url, f"/workspaces/{workspace}/agents", token=token)
    for agent in data.get("agents") or []:
        if str(agent.get("name", "")).lower() == agent_name.lower():
            return str(agent["id"])
    raise SystemExit(f"agent {agent_name!r} not found in workspace {workspace!r}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Seed recommended project skills into cron-agent-dashboard")
    parser.add_argument("--base-url", default=os.environ.get("DASHBOARD_API", "http://127.0.0.1:8080/api"))
    parser.add_argument("--workspace", default="", help="Workspace slug or id. Defaults to payload workspace_slug.")
    parser.add_argument("--agent-id", default="", help="Agent id to assign skills to.")
    parser.add_argument("--agent-name", default="", help="Agent name to resolve and assign skills to.")
    parser.add_argument("--payload", default=str(DEFAULT_PAYLOAD))
    parser.add_argument("--token", default=os.environ.get("CRON_AGENT_DASHBOARD_TOKEN", os.environ.get("DASHBOARD_TOKEN", "")))
    parser.add_argument("--register-only", action="store_true", help="Only create/update workspace skill registry; skip assignment.")
    args = parser.parse_args()

    payload_path = Path(args.payload)
    payload = json.loads(payload_path.read_text())
    workspace = args.workspace or payload.get("workspace_slug")
    if not workspace:
        raise SystemExit("--workspace is required when payload has no workspace_slug")

    created_by_name: dict[str, str] = {}
    for skill in payload.get("skills", []):
        body = dict(skill)
        result = request("POST", args.base_url, f"/workspaces/{workspace}/skills", body, token=args.token)
        created = result.get("skill") or {}
        created_by_name[created.get("name") or body.get("name")] = created.get("id")
        print(f"registered skill: {created.get('name')} ({created.get('id')})")

    if args.register_only:
        return 0

    agent_id = resolve_agent_id(args.base_url, workspace, args.agent_id, args.agent_name, args.token)
    if not agent_id:
        print("no --agent-id/--agent-name provided; skipped assignment", file=sys.stderr)
        return 0

    for assignment in payload.get("recommended_agent_assignment", []):
        skill_name = assignment["skill_name"]
        skill_id = created_by_name.get(skill_name)
        if not skill_id:
            raise SystemExit(f"skill {skill_name!r} was not created")
        body = {
            "skill_id": skill_id,
            "activation_mode": assignment.get("activation_mode", "trigger"),
            "priority": assignment.get("priority", 100),
            "enabled": assignment.get("enabled", True),
        }
        request("POST", args.base_url, f"/agents/{agent_id}/skills", body, token=args.token)
        print(f"assigned skill: {skill_name} -> agent {agent_id}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
