#!/bin/bash

# Always exit on errors.
set -e

# Set our known directories.
CNI_CONF_DIR="/host/etc/cni/net.d"
CNI_BIN_DIR="/host/opt/cni/bin"
MULTUS_CONF_FILE="/usr/src/multus-cni/images/70-multus.conf"
MULTUS_BIN_FILE="/usr/src/multus-cni/bin/multus"

# Give help text for parameters.
function usage()
{
    echo -e "This is an entrypoint script for Multus CNI to overlay its"
    echo -e "binary and configuration into locations in a filesystem."
    echo -e "The configuration & binary file will be copied to the "
    echo -e "corresponding configuration directory."
    echo -e ""
    echo -e "./entrypoint.sh"
    echo -e "\t-h --help"
    echo -e "\t--cni-conf-dir=$CNI_CONF_DIR"
    echo -e "\t--cni-bin-dir=$CNI_BIN_DIR"
    echo -e "\t--multus-conf-file=$MULTUS_CONF_FILE"
    echo -e "\t--multus-bin-file=$MULTUS_BIN_FILE"
}

# Parse parameters given as arguments to this script.
while [ "$1" != "" ]; do
    PARAM=`echo $1 | awk -F= '{print $1}'`
    VALUE=`echo $1 | awk -F= '{print $2}'`
    case $PARAM in
        -h | --help)
            usage
            exit
            ;;
        --cni-conf-dir)
            CNI_CONF_DIR=$VALUE
            ;;
        --cni-bin-dir)
            CNI_BIN_DIR=$VALUE
            ;;
        --multus-conf-file)
            MULTUS_CONF_FILE=$VALUE
            ;;
        --multus-bin-file)
            MULTUS_BIN_FILE=$VALUE
            ;;
        *)
            echo "ERROR: unknown parameter \"$PARAM\""
            usage
            exit 1
            ;;
    esac
    shift
done


# Create array of known locations
declare -a arr=($CNI_CONF_DIR $CNI_BIN_DIR $MULTUS_CONF_FILE $MULTUS_BIN_FILE)

# Loop through and verify each location each.
for i in "${arr[@]}"
do
  if [ ! -e "$i" ]; then
    echo "Location $i does not exist"
    exit 1;
  fi
done

# Copy files into proper places.
cp -f $MULTUS_CONF_FILE $CNI_CONF_DIR
cp -f $MULTUS_BIN_FILE $CNI_BIN_DIR

# Make a multus.d directory (for our kubeconfig)

mkdir -p $CNI_CONF_DIR/multus.d
MULTUS_KUBECONFIG=$CNI_CONF_DIR/multus.d/multus.kubeconfig

# ------------------------------- Generate a "kube-config"
# Inspired by: https://tinyurl.com/y7r2knme
SERVICE_ACCOUNT_PATH=/var/run/secrets/kubernetes.io/serviceaccount
KUBE_CA_FILE=${KUBE_CA_FILE:-$SERVICE_ACCOUNT_PATH/ca.crt}
SERVICEACCOUNT_TOKEN=$(cat $SERVICE_ACCOUNT_PATH/token)
SKIP_TLS_VERIFY=${SKIP_TLS_VERIFY:-false}


# Check if we're running as a k8s pod.
if [ -f "$SERVICE_ACCOUNT_PATH/token" ]; then
  # We're running as a k8d pod - expect some variables.
  if [ -z ${KUBERNETES_SERVICE_HOST} ]; then
    echo "KUBERNETES_SERVICE_HOST not set"; exit 1;
  fi
  if [ -z ${KUBERNETES_SERVICE_PORT} ]; then
    echo "KUBERNETES_SERVICE_PORT not set"; exit 1;
  fi

  if [ "$SKIP_TLS_VERIFY" == "true" ]; then
    TLS_CFG="insecure-skip-tls-verify: true"
  elif [ -f "$KUBE_CA_FILE" ]; then
    TLS_CFG="certificate-authority-data: $(cat $KUBE_CA_FILE | base64 | tr -d '\n')"
  fi

  # Write a kubeconfig file for the CNI plugin.  Do this
  # to skip TLS verification for now.  We should eventually support
  # writing more complete kubeconfig files. This is only used
  # if the provided CNI network config references it.
  touch $MULTUS_KUBECONFIG
  chmod ${KUBECONFIG_MODE:-600} $MULTUS_KUBECONFIG
  cat > $MULTUS_KUBECONFIG <<EOF
# Kubeconfig file for Multus CNI plugin.
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: ${KUBERNETES_SERVICE_PROTOCOL:-https}://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}
    $TLS_CFG
users:
- name: multus
  user:
    token: "${SERVICEACCOUNT_TOKEN}"
contexts:
- name: multus-context
  context:
    cluster: local
    user: multus
current-context: multus-context
EOF

else
  echo "WARNING: Doesn't look like we're running in a kubernetes environment (no serviceaccount token)"
fi

# ---------------------- end Generate a "kube-config".

echo "Entering sleep... (success)"

# Sleep forever.
sleep infinity
