#!/usr/bin/env python3
# SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
# SPDX-License-Identifier: Apache-2.0
#
# Adapted from ovn-kubernetes contrib/perf for Multus CI reporting.

"""
Download artifacts from a GitHub Actions workflow run.

Usage:
    python download-artifacts.py --run-id 123456 --filter "performance-test-data" --output-dir ./artifacts
"""

import argparse
import sys
import zipfile
from pathlib import Path

import requests
from github_common import get_github_token, get_repo_info


def list_artifacts(owner: str, repo: str, run_id: int, token: str) -> list[dict]:
    """List all artifacts for a workflow run."""
    url = f"https://api.github.com/repos/{owner}/{repo}/actions/runs/{run_id}/artifacts"
    headers = {
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github.v3+json'
    }

    response = requests.get(url, headers=headers, timeout=30)
    response.raise_for_status()

    data = response.json()
    return data.get('artifacts', [])


def download_artifact(owner: str, repo: str, artifact_id: int, output_path: Path, token: str) -> bool:
    """Download a single artifact."""
    url = f"https://api.github.com/repos/{owner}/{repo}/actions/artifacts/{artifact_id}/zip"
    headers = {
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github.v3+json'
    }

    print(f"  Downloading artifact {artifact_id}...", file=sys.stderr)

    response = requests.get(url, headers=headers, stream=True, timeout=30)
    response.raise_for_status()

    with open(output_path, 'wb') as f:
        for chunk in response.iter_content(chunk_size=8192):
            f.write(chunk)

    return True


def extract_artifact(zip_path: Path, extract_to: Path) -> None:
    """Extract a zip artifact, rejecting path-traversal members."""
    print(f"  Extracting to {extract_to}...", file=sys.stderr)
    extract_to = extract_to.resolve()
    extract_to.mkdir(parents=True, exist_ok=True)

    with zipfile.ZipFile(zip_path, 'r') as zip_ref:
        for member in zip_ref.infolist():
            member_path = Path(member.filename)
            # Reject absolute paths, drive letters, and ".." traversal.
            if (
                member_path.is_absolute()
                or member_path.anchor
                or '..' in member_path.parts
            ):
                raise ValueError(f"Unsafe ZIP member path: {member.filename!r}")

            target = (extract_to / member_path).resolve()
            if not target.is_relative_to(extract_to):
                raise ValueError(
                    f"ZIP member escapes extract dir: {member.filename!r}"
                )

            zip_ref.extract(member, extract_to)


def main():
    parser = argparse.ArgumentParser(description='Download GitHub Actions workflow artifacts')
    parser.add_argument('--run-id', required=True, type=int, help='Workflow run ID')
    parser.add_argument('--filter', help='Filter artifacts by name (substring match)')
    parser.add_argument('--output-dir', type=Path, default=Path('.'), help='Output directory')
    parser.add_argument('--extract', action='store_true', help='Automatically extract zip files')
    parser.add_argument('--owner', help='Repository owner (default: auto-detect)')
    parser.add_argument('--repo', help='Repository name (default: auto-detect)')

    args = parser.parse_args()

    # Get repo info
    if args.owner and args.repo:
        owner, repo = args.owner, args.repo
    else:
        owner, repo = get_repo_info(allow_git_fallback=True)

    print(f"Repository: {owner}/{repo}", file=sys.stderr)
    print(f"Workflow Run ID: {args.run_id}", file=sys.stderr)

    token = get_github_token()

    # List artifacts
    print("Fetching artifacts list...", file=sys.stderr)
    artifacts = list_artifacts(owner, repo, args.run_id, token)
    print(f"Found {len(artifacts)} total artifacts", file=sys.stderr)

    # Filter artifacts
    if args.filter:
        artifacts = [a for a in artifacts if args.filter in a['name']]
        print(f"Filtered to {len(artifacts)} artifacts matching '{args.filter}'", file=sys.stderr)

    if not artifacts:
        print("No artifacts to download", file=sys.stderr)
        return 0

    # Create output directory
    args.output_dir.mkdir(parents=True, exist_ok=True)

    # Download artifacts
    downloaded = []
    for artifact in artifacts:
        artifact_name = artifact['name']
        artifact_id = artifact['id']

        print(f"Processing: {artifact_name}", file=sys.stderr)

        zip_path = args.output_dir / f"{artifact_name}.zip"

        try:
            download_artifact(owner, repo, artifact_id, zip_path, token)
            downloaded.append(str(zip_path))

            if args.extract:
                # Extract to subdirectory named after artifact
                extract_dir = args.output_dir / artifact_name
                extract_artifact(zip_path, extract_dir)
                print(f"  Extracted to: {extract_dir}", file=sys.stderr)

        except Exception as e:
            print(f"  Error downloading {artifact_name}: {e}", file=sys.stderr)
            continue

    print(f"\nSuccessfully downloaded {len(downloaded)} artifact(s)", file=sys.stderr)

    # Output downloaded file paths (one per line) for workflow consumption
    for path in downloaded:
        print(path)

    return 0


if __name__ == '__main__':
    sys.exit(main())
