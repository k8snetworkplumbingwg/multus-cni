#!/usr/bin/env python3
# SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
# SPDX-License-Identifier: Apache-2.0
#
# Adapted from ovn-kubernetes contrib/perf for Multus CI reporting.

"""
Compare performance reports between current PR run and baseline.

This script works with structured JSON data files instead of parsing markdown.
It compares metrics and generates markdown reports with comparison tables.

Usage:
    python compare-reports.py --current data.json --baseline baseline.json --output report.md
"""

import argparse
import json
import sys
from pathlib import Path


def calc_delta(current: float | None, baseline: float | None, decimals: int = 1) -> dict:
    """
    Calculate delta and percentage change with safe division.

    Returns dict with absolute and percentage changes, or None values if invalid.
    """
    if current is None or baseline is None or baseline == 0:
        return {'absolute': None, 'percent': None, 'display': 'N/A'}

    delta = current - baseline
    percent = (delta / baseline) * 100

    sign = '+' if delta > 0 else ''
    percent_sign = '+' if percent > 0 else ''

    return {
        'absolute': delta,
        'percent': percent,
        'display': f"{sign}{delta:.{decimals}f} ({percent_sign}{percent:.1f}%)"
    }


def compare_latency_metrics(current: dict | None, baseline: dict | None) -> dict:
    """Compare pod ready latency metrics."""
    if not current:
        return {'current': None, 'baseline': baseline, 'deltas': {}}

    if not baseline:
        return {'current': current, 'baseline': None, 'deltas': {}}

    deltas = {}
    for metric in ['avg_latency', 'max_latency', 'min_latency']:
        if metric in current and metric in baseline:
            decimals = 1 if metric == 'avg_latency' else 0
            deltas[metric] = calc_delta(current[metric], baseline[metric], decimals)

    # Handle total_pods separately (integer delta, no percentage)
    if 'total_pods' in current and 'total_pods' in baseline:
        delta = current['total_pods'] - baseline['total_pods']
        sign = '+' if delta > 0 else ''
        deltas['total_pods'] = {
            'absolute': delta,
            'percent': None,
            'display': f"{sign}{delta}"
        }

    return {
        'current': current,
        'baseline': baseline,
        'deltas': deltas
    }


def compare_resource_metrics(current: dict, baseline: dict) -> dict:
    """Compare CPU or memory metrics across container types."""
    comparison = {}

    # Get all container types from both current and baseline
    all_types = set(list(current.keys()) + list(baseline.keys()))

    for container_type in all_types:
        curr_data = current.get(container_type)
        base_data = baseline.get(container_type)

        if curr_data and base_data:
            # Both exist - calculate deltas
            comparison[container_type] = {
                'current': curr_data,
                'baseline': base_data,
                'deltas': {
                    'avg': calc_delta(curr_data['avg'], base_data['avg'], 2),
                    'max': calc_delta(curr_data['max'], base_data['max'], 2)
                }
            }
        elif curr_data:
            # Only in current
            comparison[container_type] = {
                'current': curr_data,
                'baseline': None,
                'deltas': {}
            }
        else:
            # Only in baseline
            comparison[container_type] = {
                'current': None,
                'baseline': base_data,
                'deltas': {}
            }

    return comparison


def add_baseline_comparison(current_data: dict, baseline_data: dict | None, baseline_url: str = None) -> dict:
    """
    Add baseline comparison to performance data.

    Args:
        current_data: Current PR performance data (JSON)
        baseline_data: Baseline performance data (JSON)
        baseline_url: Optional URL to baseline workflow run

    Returns:
        Enhanced data with comparison information
    """
    enhanced = {
        'workload': current_data.get('workload'),
        'generated_at': current_data.get('generated_at'),
        'baseline_url': baseline_url,
        'has_baseline': baseline_data is not None
    }

    if not baseline_data:
        # No baseline - just return current data with no comparisons
        enhanced['pod_latency'] = {
            'current': current_data.get('pod_latency'),
            'baseline': None,
            'deltas': {}
        }
        enhanced['cpu'] = {k: {'current': v, 'baseline': None, 'deltas': {}}
                          for k, v in current_data.get('cpu', {}).items()}
        enhanced['memory'] = {k: {'current': v, 'baseline': None, 'deltas': {}}
                             for k, v in current_data.get('memory', {}).items()}
        return enhanced

    # Compare pod latency metrics
    enhanced['pod_latency'] = compare_latency_metrics(
        current_data.get('pod_latency'),
        baseline_data.get('pod_latency')
    )

    # Compare CPU metrics
    enhanced['cpu'] = compare_resource_metrics(
        current_data.get('cpu', {}),
        baseline_data.get('cpu', {})
    )

    # Compare memory metrics
    enhanced['memory'] = compare_resource_metrics(
        current_data.get('memory', {}),
        baseline_data.get('memory', {})
    )

    return enhanced


def render_markdown(data: dict) -> str:
    """
    Render enhanced comparison data as markdown.

    Args:
        data: Enhanced data with comparisons

    Returns:
        Markdown formatted report
    """
    lines = []

    # Header
    lines.append("# 📊 Kubernetes Workload Metrics Report")
    lines.append(f"## {data.get('workload', 'Unknown')} Performance Results")
    lines.append("")
    lines.append(f"**Generated on:** {data.get('generated_at', 'Unknown')}")
    lines.append("")

    # Baseline reference
    if data.get('has_baseline'):
        baseline_ref = f"[workflow]({data['baseline_url']})" if data.get('baseline_url') else 'N/A'
        lines.append(f"> 📊 **Baseline:** Run from {baseline_ref}")
        lines.append("")

    # Pod Ready Latency
    lines.append("## 🎯 Pod Ready Latency (Main KPI)")

    latency = data.get('pod_latency', {})
    current = latency.get('current')
    baseline = latency.get('baseline')
    deltas = latency.get('deltas', {})

    if current and baseline:
        # Comparison table
        lines.append("| Metric | Current | Baseline | Change |")
        lines.append("|--------|---------|----------|--------|")

        if 'avg_latency' in current and 'avg_latency' in baseline:
            lines.append(
                f"| Average Latency | **{current['avg_latency']:.1f} ms** | "
                f"{baseline['avg_latency']:.1f} ms | {deltas.get('avg_latency', {}).get('display', 'N/A')} |"
            )

        if 'max_latency' in current and 'max_latency' in baseline:
            lines.append(
                f"| Max Latency | **{current['max_latency']:.0f} ms** | "
                f"{baseline['max_latency']:.0f} ms | {deltas.get('max_latency', {}).get('display', 'N/A')} |"
            )

        if 'min_latency' in current and 'min_latency' in baseline:
            lines.append(
                f"| Min Latency | **{current['min_latency']:.0f} ms** | "
                f"{baseline['min_latency']:.0f} ms | {deltas.get('min_latency', {}).get('display', 'N/A')} |"
            )

        if 'total_pods' in current and 'total_pods' in baseline:
            lines.append(
                f"| Total Pods | **{current['total_pods']}** | "
                f"{baseline['total_pods']} | {deltas.get('total_pods', {}).get('display', 'N/A')} |"
            )

    elif current:
        # No baseline - simple table
        lines.append("| Metric | Value |")
        lines.append("|--------|-------|")
        if 'avg_latency' in current:
            lines.append(f"| Average Latency | **{current.get('avg_latency', 0):.1f} ms** |")
        if 'max_latency' in current:
            lines.append(f"| Max Latency | **{current.get('max_latency', 0):.0f} ms** |")
        if 'min_latency' in current:
            lines.append(f"| Min Latency | **{current.get('min_latency', 0):.0f} ms** |")
        if 'total_pods' in current:
            lines.append(f"| Total Pods | **{current.get('total_pods', 0)}** |")
    else:
        lines.append("⚠️ No pod latency data available")

    lines.append("")

    # Optional container resource usage (only when present in the JSON data)
    cpu_data = data.get('cpu', {})
    memory_data = data.get('memory', {})
    if cpu_data or memory_data:
        lines.append("## 💻 Container-Level Resource Usage")

        if cpu_data:
            lines.append("### CPU Usage Summary")

            if data.get('has_baseline'):
                lines.append("| Container Type | Avg CPU (Current) | Avg CPU (Baseline) | Change | Max CPU (Current) | Max CPU (Baseline) | Change |")
                lines.append("|----------------|-------------------|-------------------|--------|-------------------|-------------------|--------|")

                for container_type in sorted(cpu_data.keys()):
                    info = cpu_data[container_type]
                    curr = info.get('current')
                    base = info.get('baseline')
                    deltas = info.get('deltas', {})

                    if curr and base:
                        lines.append(
                            f"| {container_type} | **{curr['avg']:.2f}%** | "
                            f"{base['avg']:.2f}% | {deltas.get('avg', {}).get('display', 'N/A')} | "
                            f"**{curr['max']:.2f}%** | {base['max']:.2f}% | {deltas.get('max', {}).get('display', 'N/A')} |"
                        )
                    elif curr:
                        lines.append(
                            f"| {container_type} | **{curr['avg']:.2f}%** | - | - | "
                            f"**{curr['max']:.2f}%** | - | - |"
                        )
            else:
                lines.append("| Container Type | Avg CPU (%) | Max CPU (%) | Data Points |")
                lines.append("|----------------|-------------|-------------|-------------|")

                for container_type in sorted(cpu_data.keys()):
                    info = cpu_data[container_type]
                    curr = info.get('current')
                    if curr:
                        lines.append(
                            f"| {container_type} | {curr['avg']:.2f}% | {curr['max']:.2f}% | {curr.get('data_points', 0)} |"
                        )
            lines.append("")

        if memory_data:
            lines.append("### Memory Usage Summary")

            if data.get('has_baseline'):
                lines.append("| Container Type | Avg Memory (Current) | Avg Memory (Baseline) | Change | Max Memory (Current) | Max Memory (Baseline) | Change |")
                lines.append("|----------------|----------------------|-----------------------|--------|----------------------|-----------------------|--------|")

                for container_type in sorted(memory_data.keys()):
                    info = memory_data[container_type]
                    curr = info.get('current')
                    base = info.get('baseline')
                    deltas = info.get('deltas', {})

                    if curr and base:
                        lines.append(
                            f"| {container_type} | **{curr['avg']:.2f} MB** | "
                            f"{base['avg']:.2f} MB | {deltas.get('avg', {}).get('display', 'N/A')} | "
                            f"**{curr['max']:.2f} MB** | {base['max']:.2f} MB | {deltas.get('max', {}).get('display', 'N/A')} |"
                        )
                    elif curr:
                        lines.append(
                            f"| {container_type} | **{curr['avg']:.2f} MB** | - | - | "
                            f"**{curr['max']:.2f} MB** | - | - |"
                        )
            else:
                lines.append("| Container Type | Avg Memory (MB) | Max Memory (MB) | Data Points |")
                lines.append("|----------------|-----------------|-----------------|-------------|")

                for container_type in sorted(memory_data.keys()):
                    info = memory_data[container_type]
                    curr = info.get('current')
                    if curr:
                        lines.append(
                            f"| {container_type} | {curr['avg']:.2f} MB | {curr['max']:.2f} MB | {curr.get('data_points', 0)} |"
                        )
            lines.append("")

    lines.append("---")
    lines.append("*Report generated by Multus performance testing*")

    return '\n'.join(lines)


def main():
    parser = argparse.ArgumentParser(
        description='Compare performance reports and generate comparison markdown'
    )
    parser.add_argument(
        '--current',
        required=True,
        type=Path,
        help='Path to current performance data JSON file'
    )
    parser.add_argument(
        '--baseline',
        type=Path,
        help='Path to baseline performance data JSON file'
    )
    parser.add_argument(
        '--baseline-url',
        help='URL to baseline workflow run'
    )
    parser.add_argument(
        '--output',
        type=Path,
        required=True,
        help='Output path for markdown report'
    )

    args = parser.parse_args()

    # Read current data
    if not args.current.exists():
        print(f"Error: Current data file not found: {args.current}", file=sys.stderr)
        sys.exit(1)

    with open(args.current, 'r') as f:
        try:
            current_data = json.load(f)
        except json.JSONDecodeError as e:
            print(f"Error: Failed to parse current data JSON: {e}", file=sys.stderr)
            sys.exit(1)

    # Read baseline data if provided
    baseline_data = None
    if args.baseline:
        if not args.baseline.exists():
            print(f"Warning: Baseline data file not found: {args.baseline}", file=sys.stderr)
        else:
            with open(args.baseline, 'r') as f:
                try:
                    baseline_data = json.load(f)
                except json.JSONDecodeError as e:
                    print(f"Warning: Failed to parse baseline data JSON: {e}", file=sys.stderr)

    # Generate enhanced comparison data
    enhanced_data = add_baseline_comparison(current_data, baseline_data, args.baseline_url)

    # Generate markdown
    markdown_content = render_markdown(enhanced_data)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with open(args.output, 'w') as f:
        f.write(markdown_content)
    print(f"Markdown report written to: {args.output}", file=sys.stderr)

    return 0


if __name__ == '__main__':
    sys.exit(main())
