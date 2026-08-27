#!/usr/bin/env python3
# SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
# SPDX-License-Identifier: Apache-2.0
#
# Adapted from ovn-kubernetes contrib/perf for Multus CI reporting.

"""
Extract PR information from a GitHub workflow run.

Usage:
    python get-pr-info.py --run-id 123456
    python get-pr-info.py --event-path $GITHUB_EVENT_PATH
"""

import argparse
import json
import sys
import subprocess
from pathlib import Path
from typing import List

import requests
from github_common import get_github_token, get_repo_info


def find_prs_by_head_branch(
    head_branch: str,
    head_owner: str | None = None,
    repo: str | None = None,
) -> List[int]:
    """Find open PR numbers whose head branch matches.

    Used by workflow_run handlers to map an e2e-kind run back to the PR that
    triggered it. Pass values from the triggering run's metadata:

        head_branch  workflow_run.head_branch (e.g. "gracefulterm")
        head_owner   workflow_run.head_repository.owner.login (e.g. "wizhaoredhat")
        repo         upstream "owner/name" (e.g. "k8snetworkplumbingwg/multus-cni")

    Search by branch name only, then filter by head_owner when given. Do not pass
    "owner:branch" to gh: `gh pr list --head` documents that syntax as unsupported
    and returns no results for fork PRs.

    Returns:
        Matching PR numbers, or [] when none are found or gh fails.
    """
    if not head_branch:
        return []

    # gh pr list --head accepts the branch name only; owner:branch returns no results.
    if head_owner:
        print(
            f"Searching for PRs by head branch {head_branch!r} (owner: {head_owner})",
            file=sys.stderr,
        )
    else:
        print(f"Searching for PRs by head branch: {head_branch}", file=sys.stderr)

    cmd = [
        'gh', 'pr', 'list',
        '--head', head_branch,
        '--json', 'number,headRepositoryOwner',
    ]
    if repo:
        cmd.extend(['--repo', repo])

    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            check=True,
            timeout=30,
        )

        if not result.stdout.strip():
            return []

        prs = json.loads(result.stdout)
        if head_owner:
            prs = [
                pr for pr in prs
                if (pr.get('headRepositoryOwner') or {}).get('login') == head_owner
            ]

        pr_numbers = [int(pr['number']) for pr in prs]
        if pr_numbers:
            print(f"Found PR(s): {pr_numbers}", file=sys.stderr)
        return pr_numbers
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError, json.JSONDecodeError, KeyError, TypeError, ValueError) as e:
        print(f"Warning: gh CLI failed: {e}", file=sys.stderr)
        return []


def get_prs_from_event_file(event_path: Path) -> List[int]:
    """
    Extract PR numbers from GitHub event file.

    For pull_request events, searches for PRs matching the head_branch.
    For other events (schedule, etc.), returns empty list.
    """
    if not event_path.exists():
        print(f"Error: Event file not found: {event_path}", file=sys.stderr)
        sys.exit(1)

    with open(event_path, 'r') as f:
        event_data = json.load(f)

    workflow_run = event_data.get('workflow_run', {})

    # Only search for PRs if the triggering workflow was a pull_request event
    if workflow_run.get('event') != 'pull_request':
        return []

    head_branch = workflow_run.get('head_branch')
    if not head_branch:
        return []

    head_owner = (
        (workflow_run.get('head_repository') or {}).get('owner') or {}
    ).get('login')

    owner, name = get_repo_info()
    return find_prs_by_head_branch(head_branch, head_owner, f"{owner}/{name}")


def get_prs_from_api(owner: str, repo: str, run_id: int, token: str) -> List[int]:
    """
    Get PR numbers from workflow run via GitHub API.

    For pull_request events, searches for PRs matching the head_branch.
    For other events (schedule, etc.), returns empty list.
    """
    url = f"https://api.github.com/repos/{owner}/{repo}/actions/runs/{run_id}"
    headers = {
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github.v3+json'
    }

    response = requests.get(url, headers=headers, timeout=30)
    response.raise_for_status()

    data = response.json()

    # Only search for PRs if this was a pull_request event
    if data.get('event') != 'pull_request':
        return []

    head_branch = data.get('head_branch')
    if not head_branch:
        return []

    head_owner = (
        (data.get('head_repository') or {}).get('owner') or {}
    ).get('login')

    return find_prs_by_head_branch(head_branch, head_owner, f"{owner}/{repo}")


def main():
    parser = argparse.ArgumentParser(description='Get PR information from workflow run')
    parser.add_argument('--run-id', type=int, help='Workflow run ID')
    parser.add_argument('--event-path', type=Path, help='Path to GitHub event JSON file')
    parser.add_argument('--owner', help='Repository owner (default: from GITHUB_REPOSITORY)')
    parser.add_argument('--repo', help='Repository name (default: from GITHUB_REPOSITORY)')
    parser.add_argument('--format', choices=['json', 'space', 'newline'], default='json',
                        help='Output format (default: json)')

    args = parser.parse_args()

    pr_numbers = []

    # Try event file first (most reliable)
    if args.event_path:
        print("Extracting PR info from event file...", file=sys.stderr)
        pr_numbers = get_prs_from_event_file(args.event_path)

    # Fallback to API
    elif args.run_id:
        if args.owner and args.repo:
            owner, repo = args.owner, args.repo
        else:
            owner, repo = get_repo_info()

        token = get_github_token()

        print(f"Fetching PR info for run {args.run_id} from API...", file=sys.stderr)
        pr_numbers = get_prs_from_api(owner, repo, args.run_id, token)

    else:
        print("Error: Must provide either --run-id or --event-path", file=sys.stderr)
        sys.exit(1)

    print(f"Found {len(pr_numbers)} associated PR(s): {pr_numbers}", file=sys.stderr)

    # Output in requested format
    if args.format == 'json':
        print(json.dumps(pr_numbers))
    elif args.format == 'space':
        print(' '.join(map(str, pr_numbers)))
    elif args.format == 'newline':
        for pr in pr_numbers:
            print(pr)

    return 0


if __name__ == '__main__':
    sys.exit(main())
