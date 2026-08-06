#!/usr/bin/env python3
# SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
# SPDX-License-Identifier: Apache-2.0
#
# Adapted from ovn-kubernetes contrib/perf for Multus CI reporting.

"""
Kubernetes Workload Metrics Report Generator

Generates a markdown report from kube-burner JSON metrics files.
Focuses on podReadyLatency as the main KPI. Optional container CPU/Memory
sections are included only when those metrics files are present.
"""

import json
import os
import sys
import subprocess
from datetime import datetime, timezone
from typing import Dict, List, Any, Optional
import argparse


class MetricsProcessor:
    """Process and analyze metrics data from JSON files."""

    def __init__(self, metrics_dir: str = ".", workload: str = "kubelet-density-cni"):
        self.workload = workload
        self.metrics_dir = metrics_dir
        self.pod_latency_file = f"podLatencyMeasurement-{self.workload}.json"
        self.container_cpu_file = "containerCPU.json"
        self.container_memory_file = "containerMemory.json"

    def load_json_file(self, filename: str) -> List[Dict[str, Any]]:
        """Load and parse JSON file (array or NDJSON)."""
        filepath = os.path.join(self.metrics_dir, filename)
        try:
            with open(filepath, 'r') as f:
                raw = f.read().strip()
            if not raw:
                return []
            try:
                data = json.loads(raw)
            except json.JSONDecodeError:
                # kube-burner local indexer may emit NDJSON
                data = [json.loads(line) for line in raw.splitlines() if line.strip()]
            if isinstance(data, dict):
                data = [data]
            print(f"✓ Loaded {len(data)} records from {filename}")
            return data
        except FileNotFoundError:
            print(f"✗ Error: {filename} not found in {self.metrics_dir}")
            return []
        except json.JSONDecodeError as e:
            print(f"✗ Error parsing {filename}: {e}")
            return []

    def process_pod_latency_data(self, data: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Process pod latency data and calculate statistics."""
        valid_data = [
            d for d in data
            if d.get('podReadyLatency') is not None and d.get('timestamp')
        ]

        if not valid_data:
            return {"data": [], "stats": {}}

        # Sort by timestamp
        valid_data.sort(key=lambda x: x['timestamp'])

        # Calculate statistics
        latencies = [d['podReadyLatency'] for d in valid_data]
        stats = {
            "total_pods": len(valid_data),
            "avg_latency": sum(latencies) / len(latencies),
            "max_latency": max(latencies),
            "min_latency": min(latencies),
            "start_time": valid_data[0]['timestamp'],
            "end_time": valid_data[-1]['timestamp']
        }

        return {
            "data": valid_data,
            "stats": stats
        }

    def process_ovn_data(self, data: List[Dict[str, Any]], metric_type: str) -> Dict[str, List[Dict[str, Any]]]:
        """Process OVN container CPU/Memory data."""
        ovn_data = {}

        for record in data:
            labels = record.get('labels', {})
            pod_name = labels.get('pod', '')
            container_name = labels.get('container', '')

            # Filter for OVN containers
            if not any(keyword in pod_name for keyword in ['ovnkube-', 'ovs-']):
                continue

            # Categorize container type based on container name
            container_type = self.get_container_type_from_container(container_name, pod_name)

            if container_type not in ovn_data:
                ovn_data[container_type] = []

            # Convert memory to MB if needed
            value = record.get('value', 0)
            if metric_type == 'memory':
                value = value / (1024 * 1024)  # Convert bytes to MB

            ovn_data[container_type].append({
                "timestamp": record.get('timestamp'),
                "value": value,
                "pod": pod_name,
                "container": container_name,
                "node": labels.get('node', 'unknown')
            })

        # Sort each container type by timestamp
        for container_type in ovn_data:
            ovn_data[container_type].sort(key=lambda x: x['timestamp'])

        return ovn_data

    def get_container_type_from_container(self, container_name: str, pod_name: str) -> str:
        """Categorize OVN container types based on container name."""
        if container_name == 'ovnkube-cluster-manager':
            return 'OVNKube Cluster Manager'
        elif container_name == 'ovnkube-identity':
            return 'OVNKube Identity'
        elif container_name == 'ovnkube-controller':
            return 'OVNKube Controller'
        elif container_name == 'ovn-controller':
            return 'OVN Controller'
        elif container_name == 'ovn-northd':
            return 'OVN Northd'
        elif container_name in ['nb-ovsdb', 'sb-ovsdb']:
            return f'OVSDB ({container_name})'
        elif container_name == 'ovs-daemons':
            return 'OVS Daemons'
        elif container_name == 'ovs-metrics-exporter':
            return 'OVS Metrics Exporter'
        else:
            # Fallback to pod-based categorization
            if 'ovnkube-node' in pod_name:
                return f'OVNKube Node ({container_name})'
            elif 'ovnkube-control-plane' in pod_name:
                return f'OVNKube Control Plane ({container_name})'
            elif 'ovs-node' in pod_name:
                return f'OVS Node ({container_name})'
            else:
                return f'Other OVN ({container_name})'


class ReportGenerator:
    """Generate text report from processed metrics data."""

    def __init__(self, title: str = "Kubernetes Workload Metrics Report", workload: str = "kubelet-density-cni"):
        self.title = title
        self.workload = workload

    def generate_report(self, pod_latency: Dict[str, Any], ovn_cpu: Dict[str, Any],
                       ovn_memory: Dict[str, Any] ) -> str:
        """Generate complete text report."""

        stats = pod_latency['stats']
        report_lines = []

        # Header
        report_lines.append("# 📊 Kubernetes Workload Metrics Report")
        report_lines.append(f"## {self.workload} Performance Results")
        report_lines.append("")
        report_lines.append(f"**Generated on:** {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC')}")
        report_lines.append("")

        # Main KPI: Pod Ready Latency
        report_lines.append("## 🎯 Pod Ready Latency (Main KPI)")
        if stats:
            report_lines.append("| Metric | Value |")
            report_lines.append("|--------|-------|")
            report_lines.append(f"| Average Latency | **{stats.get('avg_latency', 0):.1f} ms** |")
            report_lines.append(f"| Max Latency | **{stats.get('max_latency', 0):.0f} ms** |")
            report_lines.append(f"| Min Latency | **{stats.get('min_latency', 0):.0f} ms** |")
            report_lines.append(f"| Total Pods | **{stats.get('total_pods', 0)}** |")
            report_lines.append(f"| Time Range | {self._format_time_range(stats.get('start_time'), stats.get('end_time'))} |")
        else:
            report_lines.append("⚠️ No pod latency data available")
        report_lines.append("")

        # Optional container resource summary (omitted when metrics are absent)
        if ovn_cpu or ovn_memory:
            report_lines.append("## 💻 Container-Level Resource Usage")

            if ovn_cpu:
                report_lines.append("### CPU Usage Summary")
                report_lines.append("| Container Type | Avg CPU (%) | Max CPU (%) | Data Points |")
                report_lines.append("|----------------|-------------|-------------|-------------|")

                for container_type, data in sorted(ovn_cpu.items()):
                    if data:
                        cpu_values = [d['value'] for d in data]
                        avg_cpu = sum(cpu_values) / len(cpu_values)
                        max_cpu = max(cpu_values)
                        report_lines.append(f"| {container_type} | {avg_cpu:.2f}% | {max_cpu:.2f}% | {len(data)} |")
                report_lines.append("")

            if ovn_memory:
                report_lines.append("### Memory Usage Summary")
                report_lines.append("| Container Type | Avg Memory (MB) | Max Memory (MB) | Data Points |")
                report_lines.append("|----------------|-----------------|-----------------|-------------|")

                for container_type, data in sorted(ovn_memory.items()):
                    if data:
                        memory_values = [d['value'] for d in data]
                        avg_memory = sum(memory_values) / len(memory_values)
                        max_memory = max(memory_values)
                        report_lines.append(f"| {container_type} | {avg_memory:.2f} MB | {max_memory:.2f} MB | {len(data)} |")
                report_lines.append("")

        report_lines.append("---")
        report_lines.append("*Report generated by Multus performance testing*")

        return "\n".join(report_lines)

    def _format_time_range(self, start_time: Optional[str], end_time: Optional[str]) -> str:
        """Format time range for display."""
        if not start_time or not end_time:
            return "-"

        try:
            start = datetime.fromisoformat(start_time.replace('Z', '+00:00'))
            end = datetime.fromisoformat(end_time.replace('Z', '+00:00'))

            if start.date() == end.date():
                return f"{start.strftime('%m/%d/%Y')}"
            else:
                return f"{start.strftime('%m/%d')} - {end.strftime('%m/%d')}"
        except:
            return "-"

    def post_github_comment(self, report_content: str, pr_number: str) -> bool:
        """Post report as GitHub comment using gh CLI."""
        try:
            # Use gh CLI to post comment
            cmd = ['gh', 'pr', 'comment', pr_number, '--body', report_content]
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)

            if result.returncode == 0:
                print(f"✓ Posted performance report as comment to PR #{pr_number}")
                return True
            else:
                print(f"✗ Failed to post GitHub comment: {result.stderr}")
                return False
        except Exception as e:
            print(f"✗ Error posting GitHub comment: {e}")
            return False

    def save_report(self, report_content: str, output_file: str) -> None:
        """Save report to file."""
        with open(output_file, 'w') as f:
            f.write(report_content)
        print(f"✓ Report saved to: {output_file}")

    def save_json_data(self, pod_latency: dict[str, Any], ovn_cpu: dict[str, Any],
                       ovn_memory: dict[str, Any], output_file: str) -> None:
        """Save structured data as JSON for downstream processing."""
        # Build CPU summary
        cpu_summary = {}
        for container_type, data in ovn_cpu.items():
            if data:
                cpu_values = [d['value'] for d in data]
                cpu_summary[container_type] = {
                    'avg': sum(cpu_values) / len(cpu_values),
                    'max': max(cpu_values),
                    'data_points': len(data)
                }

        # Build memory summary
        memory_summary = {}
        for container_type, data in ovn_memory.items():
            if data:
                memory_values = [d['value'] for d in data]
                memory_summary[container_type] = {
                    'avg': sum(memory_values) / len(memory_values),
                    'max': max(memory_values),
                    'data_points': len(data)
                }

        # Build structured output
        output_data = {
            'workload': self.workload,
            'generated_at': datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC'),
            'pod_latency': pod_latency['stats'] if pod_latency['stats'] else None,
            'cpu': cpu_summary,
            'memory': memory_summary
        }

        with open(output_file, 'w') as f:
            json.dump(output_data, f, indent=2)
        print(f"✓ JSON data saved to: {output_file}")


def detect_pr_environment() -> Optional[str]:
    """Detect if running in a PR environment and return PR number."""
    # Check for GitHub Actions PR environment
    github_event_name = os.environ.get('GITHUB_EVENT_NAME')
    github_ref = os.environ.get('GITHUB_REF')

    if github_event_name == 'pull_request':
        # Extract PR number from GITHUB_REF (e.g., "refs/pull/123/merge")
        if github_ref and 'pull' in github_ref:
            try:
                pr_number = github_ref.split('/')[2]
                return pr_number
            except (IndexError, ValueError):
                pass

    # Check for GITHUB_EVENT_PATH which contains PR info
    event_path = os.environ.get('GITHUB_EVENT_PATH')
    if event_path and os.path.exists(event_path):
        try:
            with open(event_path, 'r') as f:
                event_data = json.load(f)
                if 'pull_request' in event_data:
                    return str(event_data['pull_request']['number'])
        except (json.JSONDecodeError, KeyError, FileNotFoundError):
            pass

    # Check for manual PR number in environment
    pr_number = os.environ.get('PR_NUMBER')
    if pr_number:
        return pr_number

    return None




def main():
    """Main function to generate the performance report."""
    parser = argparse.ArgumentParser(description='Generate Kubernetes workload metrics report')
    parser.add_argument('--workload', default='kubelet-density-cni',
                       help='Workload name (default: kubelet-density-cni)')
    parser.add_argument('--metrics-dir', default='.',
                       help='Directory containing JSON metrics files (default: current directory)')
    parser.add_argument('--output', default='performance_report.md',
                       help='Output file name (default: performance_report.md)')
    parser.add_argument('--json-output',
                       help='Output file for structured JSON data')
    parser.add_argument('--title', default='Kubernetes Workload Metrics Report',
                       help='Report title')
    parser.add_argument('--pr-number',
                       help='PR number for GitHub comment (overrides auto-detection)')
    parser.add_argument('--github-comment', action='store_true',
                       help='Post report as GitHub comment if PR detected')

    args = parser.parse_args()

    print(f"🚀 Generating Kubernetes Workload Metrics Report")
    print(f"📁 Metrics directory: {args.metrics_dir}")
    print(f"📄 Output file: {args.output}")
    print()

    # Initialize processor and generator
    processor = MetricsProcessor(args.metrics_dir, args.workload)
    generator = ReportGenerator(args.title, args.workload)

    # Load and process data
    print("📊 Loading and processing metrics data...")

    # Process pod latency data (main KPI)
    pod_latency_raw = processor.load_json_file(processor.pod_latency_file)
    pod_latency_processed = processor.process_pod_latency_data(pod_latency_raw)

    # Process OVN CPU data
    container_cpu_raw = processor.load_json_file(processor.container_cpu_file)
    ovn_cpu_processed = processor.process_ovn_data(container_cpu_raw, 'cpu')

    # Process OVN Memory data
    container_memory_raw = processor.load_json_file(processor.container_memory_file)
    ovn_memory_processed = processor.process_ovn_data(container_memory_raw, 'memory')

    print()
    print("📈 Data Processing Summary:")
    print(f"   Pod Latency Records: {len(pod_latency_processed['data'])}")
    print(f"   OVN CPU Container Types: {len(ovn_cpu_processed)}")
    print(f"   OVN Memory Container Types: {len(ovn_memory_processed)}")

    if pod_latency_processed['stats']:
        stats = pod_latency_processed['stats']
        print(f"   Average Pod Ready Latency: {stats['avg_latency']:.1f}ms")
        print(f"   Max Pod Ready Latency: {stats['max_latency']:.0f}ms")

    print()

    # Generate text report
    print("📝 Generating performance report...")
    report_content = generator.generate_report(
        pod_latency_processed,
        ovn_cpu_processed,
        ovn_memory_processed
    )

    # Save report to file
    generator.save_report(report_content, args.output)

    # Save JSON data if requested
    if args.json_output:
        generator.save_json_data(
            pod_latency_processed,
            ovn_cpu_processed,
            ovn_memory_processed,
            args.json_output
        )

    # Check for PR environment and post GitHub comment if requested
    pr_number = args.pr_number or detect_pr_environment()

    if args.github_comment and pr_number:
        print(f"\n💬 Detected PR #{pr_number}, posting GitHub comment...")
        success = generator.post_github_comment(report_content, pr_number)
        if not success:
            print("⚠️  GitHub comment failed, but report was saved to file")
    elif args.github_comment:
        print("\n⚠️  --github-comment specified but no PR detected")
        print("   Set PR_NUMBER environment variable or use --pr-number")
    elif pr_number:
        print(f"\n💡 PR #{pr_number} detected. Use --github-comment to post report as comment")

    print()
    print("🎉 Report generation complete!")
    print(f"📄 Report saved to: {args.output}")


if __name__ == "__main__":
    main()
