#!/bin/bash

# Always exit on errors.
set -e

# Set our known directories.
CNI_CONF_DIR="/host/etc/cni/net.d"
CNI_BIN_DIR="/host/opt/cni/bin"
MULTUS_CONF_FILE="/usr/src/multus-cni/images/70-multus.conf"
MULTUS_BIN_FILE="/usr/src/multus-cni/bin/multus"
MULTUS_KUBECONFIG_FILE_HOST="/etc/cni/net.d/multus.d/multus.kubeconfig"
MULTUS_NAMESPACE_ISOLATION=false
MULTUS_LOG_LEVEL=""
MULTUS_LOG_FILE=""

# Give help text for parameters.
function usage()
{
    echo -e "This is an entrypoint script for Multus CNI to overlay its binary and "
    echo -e "configuration into locations in a filesystem. The configuration & binary file "
    echo -e "will be copied to the corresponding configuration directory. When "
    echo -e "'--multus-conf-file=auto' is used, 00-multus.conf will be automatically "
    echo -e "generated from the CNI configuration file of the master plugin (the first file "
    echo -e "in lexicographical order in cni-conf-dir)."
    echo -e ""
    echo -e "./entrypoint.sh"
    echo -e "\t-h --help"
    echo -e "\t--cni-conf-dir=$CNI_CONF_DIR"
    echo -e "\t--cni-bin-dir=$CNI_BIN_DIR"
    echo -e "\t--multus-conf-file=$MULTUS_CONF_FILE"
    echo -e "\t--multus-bin-file=$MULTUS_BIN_FILE"
    echo -e "\t--multus-kubeconfig-file-host=$MULTUS_KUBECONFIG_FILE_HOST"
    echo -e "\t--namespace-isolation=$MULTUS_NAMESPACE_ISOLATION"
    echo -e "\t--multus-log-level=$MULTUS_LOG_LEVEL (empty by default, used only with --multus-conf-file=auto)"
    echo -e "\t--multus-log-file=$MULTUS_LOG_FILE (empty by default, used only with --multus-conf-file=auto)"
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
        --multus-kubeconfig-file-host)
            MULTUS_KUBECONFIG_FILE_HOST=$VALUE
            ;;
        --namespace-isolation)
            MULTUS_NAMESPACE_ISOLATION=$VALUE
            ;;
        --multus-log-level)
            MULTUS_LOG_LEVEL=$VALUE
            ;;
        --multus-log-file)
            MULTUS_LOG_FILE=$VALUE
            ;;
        *)
            echo "WARNING: unknown parameter \"$PARAM\""
            ;;
    esac
    shift
done


# Create array of known locations
declare -a arr=($CNI_CONF_DIR $CNI_BIN_DIR $MULTUS_BIN_FILE)
if [ "$MULTUS_CONF_FILE" != "auto" ]; then
  arr+=($MULTUS_CONF_FILE)
fi


# Loop through and verify each location each.
for i in "${arr[@]}"
do
  if [ ! -e "$i" ]; then
    echo "Location $i does not exist"
    exit 1;
  fi
done

# Copy files into place and atomically move into final binary name
cp -f $MULTUS_BIN_FILE $CNI_BIN_DIR/_multus
mv -f $CNI_BIN_DIR/_multus $CNI_BIN_DIR/multus 
if [ "$MULTUS_CONF_FILE" != "auto" ]; then
  cp -f $MULTUS_CONF_FILE $CNI_CONF_DIR
fi

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

# ------------------------------- Generate "00-multus.conf"

if [ "$MULTUS_CONF_FILE" == "auto" ]; then
  echo "Generating Multus configuration file ..."
  found_master=false
  tries=0
  while [ $found_master == false ]; do
    MASTER_PLUGIN="$(ls $CNI_CONF_DIR | grep -E '\.conf(list)?$' | grep -Ev '00-multus\.conf' | head -1)"
    if [ "$MASTER_PLUGIN" == "" ]; then
      if [ $tries -lt 600 ]; then
        if ! (($tries % 5)); then
          echo "Attemping to find master plugin configuration, attempt $tries"
        fi
        let "tries+=1"
        sleep 1;
      else
        echo "Error: Multus could not be configured: no master plugin was found."
        exit 1;
      fi
    else

      found_master=true

      ISOLATION_STRING=""
      if [ "$MULTUS_NAMESPACE_ISOLATION" == true ]; then
        ISOLATION_STRING="\"namespaceIsolation\": true,"
      fi

      LOG_LEVEL_STRING=""
      if [ ! -z "${MULTUS_LOG_LEVEL// }" ]; then
        case "$MULTUS_LOG_LEVEL" in
          debug)
              ;;
          error)
              ;;
          panic)
              ;;
          verbose)
              ;;
          *)
              echo "ERROR: Log levels should be one of: debug/verbose/error/panic, did not understand $MULTUS_LOG_LEVEL"
              usage
              exit 1     
        esac
        LOG_LEVEL_STRING="\"logLevel\": \"$MULTUS_LOG_LEVEL\","
      fi

      LOG_FILE_STRING=""
      if [ ! -z "${MULTUS_LOG_FILE// }" ]; then
        LOG_FILE_STRING="\"logFile\": \"$MULTUS_LOG_FILE\","
      fi

      MASTER_PLUGIN_JSON="$(cat $CNI_CONF_DIR/$MASTER_PLUGIN)"
      CONF=$(cat <<-EOF
  			{
  				"name": "multus-cni-network",
  				"type": "multus",
          $ISOLATION_STRING
          $LOG_LEVEL_STRING
          $LOG_FILE_STRING
  				"kubeconfig": "$MULTUS_KUBECONFIG_FILE_HOST",
  				"delegates": [
  					$MASTER_PLUGIN_JSON
  				]
  			}
EOF
  		)
      echo $CONF > $CNI_CONF_DIR/00-multus.conf
      echo "Config file created @ $CNI_CONF_DIR/00-multus.conf"
    fi
  done
fi

# ---------------------- end Generate "00-multus.conf".

echo "Entering sleep... (success)"

# Sleep forever.
sleep infinity
