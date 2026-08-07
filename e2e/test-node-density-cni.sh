#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

# CI-sized density: each iteration creates 2 pods (webserver + curl client).
# Default 64 pods/node on 2 workers => 64 job iterations / ~128 pods.
PODS_PER_NODE=${PODS_PER_NODE:-64}
CNI_VERSION=${CNI_VERSION:-0.4.0}
WORKLOAD=${WORKLOAD:-node-density-cni-${CNI_VERSION}}
METRICS_DIR=${METRICS_DIR:-perf-data/metrics}

# kind workers lack node-role.kubernetes.io/worker by default; macvlan master
# eth1 only exists on workers (wired by koko in setup_cluster.sh).
kubectl label node \
	-l '!node-role.kubernetes.io/control-plane' \
	node-role.kubernetes.io/worker= \
	--overwrite \
	--request-timeout=60s

WORKER_COUNT=$(kubectl get nodes -l 'node-role.kubernetes.io/worker' --no-headers --request-timeout=60s | wc -l | tr -d ' ')
if [ "${WORKER_COUNT}" -lt 1 ]; then
	echo "error: no worker nodes found for node-density-cni"
	exit 1
fi

JOB_ITERATIONS=$((WORKER_COUNT * PODS_PER_NODE / 2))
if [ "${JOB_ITERATIONS}" -lt 1 ]; then
	JOB_ITERATIONS=1
fi

rm -rf perf-data node-density-cni-metrics.tgz
mkdir -p "${METRICS_DIR}"

# Render CNI version into the NAD used by kube-burner (paths are CWD-relative).
NAD=kubeburner/templates/density-macvlan-nad.yml
NAD_BACKUP=$(mktemp)
cp "${NAD}" "${NAD_BACKUP}"
restore_nad() {
	cp "${NAD_BACKUP}" "${NAD}"
	rm -f "${NAD_BACKUP}"
}
trap restore_nad EXIT
tmp=$(mktemp)
sed "s/\"cniVersion\": \"[^\"]*\"/\"cniVersion\": \"${CNI_VERSION}\"/" "${NAD}" > "${tmp}"
if ! grep -q "\"cniVersion\": \"${CNI_VERSION}\"" "${tmp}"; then
	echo "error: cniVersion substitution failed; ${NAD} does not contain \"cniVersion\": \"${CNI_VERSION}\"" >&2
	rm -f "${tmp}"
	exit 1
fi
mv "${tmp}" "${NAD}"

echo "Running Multus node-density-cni: workers=${WORKER_COUNT} pods_per_node=${PODS_PER_NODE} job_iterations=${JOB_ITERATIONS} cni_version=${CNI_VERSION} workload=${WORKLOAD}"

# Object templates in node-density-cni.yml are relative to this working directory (e2e/).
kube-burner init -c kubeburner/node-density-cni.yml \
	--set "jobs.1.jobIterations=${JOB_ITERATIONS}"

# Normalize kube-burner local-indexer output for e2e/perf/generate_perf_report.py
TARGET="${METRICS_DIR}/podLatencyMeasurement-${WORKLOAD}.json"
if [ ! -f "${TARGET}" ]; then
	SRC=$(find "${METRICS_DIR}" -type f -name '*podLatencyMeasurement*.json' ! -name '*Quantiles*' | head -n 1 || true)
	if [ -z "${SRC}" ]; then
		echo "error: no podLatencyMeasurement metrics found under ${METRICS_DIR}"
		ls -la "${METRICS_DIR}" || true
		exit 1
	fi
	cp "${SRC}" "${TARGET}"
	echo "Normalized metrics: ${SRC} -> ${TARGET}"
fi

echo "node-density-cni completed successfully"
echo "Metrics ready for performance-report workflow under ${METRICS_DIR}"
