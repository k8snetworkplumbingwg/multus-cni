#!/usr/bin/env python3
# SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
# SPDX-License-Identifier: Apache-2.0
#
# Adapted from ovn-kubernetes contrib/perf for Multus CI reporting.

"""Shared GitHub environment and repository helpers for perf scripts."""

import os
import subprocess
import sys


def get_github_token() -> str:
    """Get GitHub token from environment."""
    token = os.getenv('GITHUB_TOKEN')
    if not token:
        print("Error: GITHUB_TOKEN environment variable not set", file=sys.stderr)
        sys.exit(1)
    return token


def get_repo_info(allow_git_fallback: bool = False) -> tuple[str, str]:
    """Get repository owner and name from environment, optionally falling back to git."""
    repo = os.getenv('GITHUB_REPOSITORY')
    if repo and '/' in repo:
        owner, name = repo.split('/', 1)
        return owner, name

    if allow_git_fallback:
        try:
            result = subprocess.run(
                ['git', 'config', '--get', 'remote.origin.url'],
                capture_output=True,
                text=True,
                check=True,
            )
            url = result.stdout.strip()
            # Parse github.com:owner/repo.git or https://github.com/owner/repo.git
            if 'github.com' in url:
                parts = url.split('github.com')[-1].strip('/:').replace('.git', '').split('/')
                if len(parts) >= 2:
                    return parts[0], parts[1]
        except subprocess.CalledProcessError:
            pass

        print("Error: Could not determine repository info", file=sys.stderr)
        sys.exit(1)

    print("Error: GITHUB_REPOSITORY environment variable not set", file=sys.stderr)
    sys.exit(1)
