#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

pod=$(kubectl -n kube-system get pods -l name=multus --field-selector spec.nodeName="${NODE_NAME:-kind-worker}" -o jsonpath='{.items[0].metadata.name}')
uid=$(kubectl -n kube-system get pod "$pod" -o jsonpath='{.metadata.uid}')

kubectl -n kube-system exec -i "$pod" -- env POD_NAME="$pod" POD_UID="$uid" sh -se <<'EOF'
plugin=/host/opt/cni/bin/multus-cancel-e2e
shim=/usr/src/multus-cni/bin/multus-shim
test ! -e "$plugin-first"
test ! -e "$plugin-second"
state=$(mktemp -d)

cleanup() {
    [ -z "${shim_pid:-}" ] || kill -KILL "$shim_pid" 2>/dev/null || true
    [ ! -s "$state/plugin.pid" ] || kill -KILL "$(cat "$state/plugin.pid")" 2>/dev/null || true
    timeout 10 env CNI_COMMAND=DEL "$shim" < "$state/config" >/dev/null 2>&1 || true
    rm -f "$plugin-first" "$plugin-second"
    rm -rf "$state"
}
trap cleanup EXIT

cat > "$plugin-first" <<PLUGIN
#!/bin/sh
if [ "\$CNI_COMMAND" = ADD ]; then
    echo \$\$ > "$state/plugin.pid"
    exec sleep 300
fi
[ "\$CNI_COMMAND" != DEL ] || touch "$state/rollback"
PLUGIN
cat > "$plugin-second" <<PLUGIN
#!/bin/sh
[ "\$CNI_COMMAND" != ADD ] || touch "$state/second.add"
echo '{"cniVersion":"0.3.1"}'
PLUGIN
chmod +x "$plugin-first" "$plugin-second"

cat > "$state/config" <<'CONFIG'
{"cniVersion":"0.3.1","name":"cancel-e2e","type":"multus-shim","daemonSocketDir":"/host/run/multus","delegates":[{"cniVersion":"0.3.1","name":"first","type":"multus-cancel-e2e-first"},{"cniVersion":"0.3.1","name":"second","type":"multus-cancel-e2e-second"}]}
CONFIG
export CNI_CONTAINERID="multus-cancel-e2e-$$" CNI_NETNS=/proc/self/ns/net CNI_IFNAME=cancel-e2e
export CNI_PATH=/host/opt/cni/bin
export CNI_ARGS="K8S_POD_NAMESPACE=kube-system;K8S_POD_NAME=$POD_NAME;K8S_POD_UID=$POD_UID"
CNI_COMMAND=ADD "$shim" < "$state/config" > "$state/shim.log" 2>&1 &
shim_pid=$!

for i in $(seq 1 100); do
    [ -s "$state/plugin.pid" ] && break
    sleep 0.1
done
if [ ! -s "$state/plugin.pid" ]; then
    echo "CNI plugin did not start"
    cat "$state/shim.log"
    exit 1
fi
plugin_pid=$(cat "$state/plugin.pid")
kill -0 "$plugin_pid"
kill -KILL "$shim_pid"
wait "$shim_pid" 2>/dev/null || true
shim_pid=

for i in $(seq 1 100); do
    if ! kill -0 "$plugin_pid" 2>/dev/null && [ -e "$state/rollback" ]; then
        test ! -e "$state/second.add"
        echo "First CNI plugin terminated after multus-shim was killed; second plugin ADD never started"
        exit 0
    fi
    sleep 0.1
done
echo "First CNI plugin did not terminate and roll back after multus-shim was killed"
exit 1
EOF
