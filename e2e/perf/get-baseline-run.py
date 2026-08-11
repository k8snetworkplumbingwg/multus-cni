#!/usr/bin/env python3
# SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
# SPDX-License-Identifier: Apache-2.0
#
# Adapted from ovn-kubernetes contrib/perf for Multus CI reporting.

"""
Find the latest successful baseline workflow run.

Usage:
    python get-baseline-run.py --workflow performance-test.yml --event schedule
"""

import argparse
import json
import sys

import requests
from github_common import get_github_token, get_repo_info


def find_baseline_run(owner: str, repo: str, workflow_id: str, event: str, token: str, limit: int = 10):
    """
    Find the latest successful workflow run for the given event type.

    Args:
        owner: Repository owner
        repo: Repository name
        workflow_id: Workflow file name (e.g., 'performance-test.yml')
        event: Event type (e.g., 'schedule' for daily runs)
        token: GitHub API token
        limit: Maximum number of runs to check

    Returns:
        dict with run info or None if not found
    """
    url = f"https://api.github.com/repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs"
    headers = {
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github.v3+json'
    }

    params = {
        'event': event,
        'status': 'success',
        'per_page': limit
    }

    print(f"Searching for baseline run (event={event}, status=success)...", file=sys.stderr)

    response = requests.get(url, headers=headers, params=params, timeout=30)
    response.raise_for_status()

    data = response.json()
    runs = data.get('workflow_runs', [])

    if not runs:
        print("No baseline run found", file=sys.stderr)
        return None

    # Return the most recent successful run
    baseline = runs[0]

    print(f"Found baseline run:", file=sys.stderr)
    print(f"  ID: {baseline['id']}", file=sys.stderr)
    print(f"  Date: {baseline['created_at']}", file=sys.stderr)
    print(f"  URL: {baseline['html_url']}", file=sys.stderr)

    return baseline


def main():
    parser = argparse.ArgumentParser(description='Find latest baseline workflow run')
    parser.add_argument('--workflow', default='performance-test.yml',
                        help='Workflow filename (default: performance-test.yml)')
    parser.add_argument('--event', default='schedule',
                        help='Event type to filter by (default: schedule)')
    parser.add_argument('--owner', help='Repository owner (default: from GITHUB_REPOSITORY)')
    parser.add_argument('--repo', help='Repository name (default: from GITHUB_REPOSITORY)')
    parser.add_argument('--limit', type=int, default=10,
                        help='Maximum number of runs to check (default: 10)')
    parser.add_argument('--output', choices=['id', 'url', 'json'], default='json',
                        help='Output format (default: json)')

    args = parser.parse_args()

    # Get repo info
    if args.owner and args.repo:
        owner, repo = args.owner, args.repo
    else:
        owner, repo = get_repo_info(allow_git_fallback=True)

    token = get_github_token()

    # Find baseline run
    baseline = find_baseline_run(owner, repo, args.workflow, args.event, token, args.limit)

    if not baseline:
        sys.exit(1)

    # Output in requested format
    if args.output == 'id':
        print(baseline['id'])
    elif args.output == 'url':
        print(baseline['html_url'])
    elif args.output == 'json':
        output = {
            'id': baseline['id'],
            'url': baseline['html_url'],
            'created_at': baseline['created_at'],
            'head_sha': baseline['head_sha']
        }
        print(json.dumps(output))

    return 0


if __name__ == '__main__':
    sys.exit(main())
