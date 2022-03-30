#!/bin/bash

# Always exit on errors.
set -e

# Trap sigterm
function exitonsigterm() {
  echo "Trapped sigterm, exiting."
  exit 0
}
trap exitonsigterm SIGTERM

# Set our known directories.
CNI_CONF_DIR="/host/etc/cni/net.d"
CNI_BIN_DIR="/host/opt/cni/bin"
ADDITIONAL_BIN_DIR=""
MULTUS_CONF_FILE="/usr/src/multus-cni/images/70-multus.conf"
MULTUS_AUTOCONF_DIR="/host/etc/cni/net.d"
MULTUS_BIN_FILE="/usr/src/multus-cni/bin/multus"
MULTUS_KUBECONFIG_FILE_HOST="/etc/cni/net.d/multus.d/multus.kubeconfig"
MULTUS_TEMP_KUBECONFIG="/tmp/multus.kubeconfig"
MULTUS_MASTER_CNI_FILE_NAME=""
MULTUS_NAMESPACE_ISOLATION=false
MULTUS_GLOBAL_NAMESPACES=""
MULTUS_LOG_TO_STDERR=true
MULTUS_LOG_LEVEL=""
MULTUS_LOG_FILE=""
MULTUS_READINESS_INDICATOR_FILE=""
OVERRIDE_NETWORK_NAME=false
MULTUS_CLEANUP_CONFIG_ON_EXIT=false
RESTART_CRIO=false
CRIO_RESTARTED_ONCE=false
RENAME_SOURCE_CONFIG_FILE=false
SKIP_BINARY_COPY=false
FORCE_CNI_VERSION=false # force-cni-version is only for e2e-kind.

# Give help text for parameters.
function usage()
{
    echo -e "This is an entrypoint script for Multus CNI to overlay its configuration into"
    echo -e "locations in a filesystem. The configuration file will be copied to the"
    echo -e "corresponding configuration directory. When '--multus-conf-file=auto' is used,"
    echo -e "00-multus.conf will be automatically generated from the CNI configuration file"
    echo -e "of the master plugin (the first file in lexicographical order in cni-conf-dir)."
    echo -e "When '--multus-master-cni-file-name' is used, 00-multus.conf will be"
    echo -e "automatically generated from the specific file rather than the first file."
    echo -e ""
    echo -e "./entrypoint.sh"
    echo -e "\t-h --help"
    echo -e "\t--cni-bin-dir=$CNI_BIN_DIR"
    echo -e "\t--cni-conf-dir=$CNI_CONF_DIR"
    echo -e "\t--cni-version=<cniVersion (e.g. 0.3.1)>"
    echo -e "\t--multus-conf-file=$MULTUS_CONF_FILE"
    echo -e "\t--multus-bin-file=$MULTUS_BIN_FILE"
    echo -e "\t--skip-multus-binary-copy=$SKIP_BINARY_COPY"
    echo -e "\t--multus-kubeconfig-file-host=$MULTUS_KUBECONFIG_FILE_HOST"
    echo -e "\t--multus-master-cni-file-name=$MULTUS_MASTER_CNI_FILE_NAME (empty by default, example: 10-calico.conflist)"
    echo -e "\t--namespace-isolation=$MULTUS_NAMESPACE_ISOLATION"
    echo -e "\t--global-namespaces=$MULTUS_GLOBAL_NAMESPACES (used only with --namespace-isolation=true)"
    echo -e "\t--multus-autoconfig-dir=$MULTUS_AUTOCONF_DIR (used only with --multus-conf-file=auto)"
    echo -e "\t--multus-log-to-stderr=$MULTUS_LOG_TO_STDERR (empty by default, used only with --multus-conf-file=auto)"
    echo -e "\t--multus-log-level=$MULTUS_LOG_LEVEL (empty by default, used only with --multus-conf-file=auto)"
    echo -e "\t--multus-log-file=$MULTUS_LOG_FILE (empty by default, used only with --multus-conf-file=auto)"
    echo -e "\t--override-network-name=false (used only with --multus-conf-file=auto)"
    echo -e "\t--cleanup-config-on-exit=false (used only with --multus-conf-file=auto)"
    echo -e "\t--rename-conf-file=false (used only with --multus-conf-file=auto)"
    echo -e "\t--readiness-indicator-file=$MULTUS_READINESS_INDICATOR_FILE (used only with --multus-conf-file=auto)"
    echo -e "\t--additional-bin-dir=$ADDITIONAL_BIN_DIR (adds binDir option to configuration, used only with --multus-conf-file=auto)"
    echo -e "\t--restart-crio=false (restarts CRIO after config file is generated)"
}

function log()
{
    echo "$(date --iso-8601=seconds) ${1}"
}

function error()
{
    log "ERR:  {$1}"
}

function warn()
{
    log "WARN: {$1}"
}

if ! type python3 &> /dev/null; then
	alias python=python3
fi

function checkCniVersion {
      cniversion_python_tmpfile=$(mktemp)
      cat << EOF > $cniversion_python_tmpfile
import json, sys

def version(v):
    return [int(x) for x in v.split(".")]

v_040 = version("0.4.0")
v_top_level = sys.argv[2]
with open(sys.argv[1], "r") as f:
    v_nested = json.load(f)["cniVersion"]
if version(v_top_level) >= v_040 and version(v_nested) < v_040:
    msg = "Multus cni version is %s while master plugin cni version is %s"
    print(msg % (v_top_level, v_nested))
EOF
      python $cniversion_python_tmpfile $1 $2
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
        --cni-version)
            CNI_VERSION=$VALUE
            ;;
	# force-cni-version is only for e2e-kind testing
	--force-cni-version)
            FORCE_CNI_VERSION=$VALUE
            ;;
        --cni-bin-dir)
            CNI_BIN_DIR=$VALUE
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
        --multus-kubeconfig-file-host)
            MULTUS_KUBECONFIG_FILE_HOST=$VALUE
            ;;
        --multus-master-cni-file-name)
            MULTUS_MASTER_CNI_FILE_NAME=$VALUE
            ;;
        --namespace-isolation)
            MULTUS_NAMESPACE_ISOLATION=$VALUE
            ;;
        --global-namespaces)
            MULTUS_GLOBAL_NAMESPACES=$VALUE
            ;;
        --multus-log-to-stderr)
            MULTUS_LOG_TO_STDERR=$VALUE
            ;;
        --multus-log-level)
            MULTUS_LOG_LEVEL=$VALUE
            ;;
        --multus-log-file)
            MULTUS_LOG_FILE=$VALUE
            ;;
        --multus-autoconfig-dir)
            MULTUS_AUTOCONF_DIR=$VALUE
            ;;
        --override-network-name)
            OVERRIDE_NETWORK_NAME=$VALUE
            ;;
        --cleanup-config-on-exit)
            MULTUS_CLEANUP_CONFIG_ON_EXIT=$VALUE
            ;;
        --restart-crio)
            RESTART_CRIO=$VALUE
            ;;
        --rename-conf-file)
            RENAME_SOURCE_CONFIG_FILE=$VALUE
            ;;
        --additional-bin-dir)
            ADDITIONAL_BIN_DIR=$VALUE
            ;;
        --skip-multus-binary-copy)
            SKIP_BINARY_COPY=$VALUE
            ;;
        --readiness-indicator-file)
            MULTUS_READINESS_INDICATOR_FILE=$VALUE
            ;;
        *)
            warn "unknown parameter \"$PARAM\""
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
    warn "Location $i does not exist"
    exit 1;
  fi
done

# Copy files into place and atomically move into final binary name
if [ "$SKIP_BINARY_COPY" = false ]; then
  cp -f $MULTUS_BIN_FILE $CNI_BIN_DIR/_multus
  mv -f $CNI_BIN_DIR/_multus $CNI_BIN_DIR/multus
else
  log "Entrypoint skipped copying Multus binary."
fi

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
    error "KUBERNETES_SERVICE_HOST not set"; exit 1;
  fi
  if [ -z ${KUBERNETES_SERVICE_PORT} ]; then
    error "KUBERNETES_SERVICE_PORT not set"; exit 1;
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
  touch $MULTUS_TEMP_KUBECONFIG
  chmod ${KUBECONFIG_MODE:-600} $MULTUS_TEMP_KUBECONFIG
  # Write the kubeconfig to a temp file first.
  cat > $MULTUS_TEMP_KUBECONFIG <<EOF
# Kubeconfig file for Multus CNI plugin.
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: ${KUBERNETES_SERVICE_PROTOCOL:-https}://[${KUBERNETES_SERVICE_HOST}]:${KUBERNETES_SERVICE_PORT}
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

  # Atomically move the temp kubeconfig to its permanent home.
  mv -f $MULTUS_TEMP_KUBECONFIG $MULTUS_KUBECONFIG

else
  warn "Doesn't look like we're running in a kubernetes environment (no serviceaccount token)"
fi

# ---------------------- end Generate a "kube-config".

# ------------------------------- Generate "00-multus.conf"

function generateMultusConf {
if [ "$MULTUS_CONF_FILE" == "auto" ]; then
  log "Generating Multus configuration file using files in $MULTUS_AUTOCONF_DIR..."
  found_master=false
  tries=0
  while [ $found_master == false ]; do
    if [ "$MULTUS_MASTER_CNI_FILE_NAME" != "" ]; then
        MASTER_PLUGIN="$MULTUS_MASTER_CNI_FILE_NAME"
	if [ ! -f "$MULTUS_AUTOCONF_DIR/$MASTER_PLUGIN" ]; then
		error "Cannot find master cni file $MULTUS_AUTOCONF_DIR/$MASTER_PLUGIN"
		exit 1;
	fi
    else
        MASTER_PLUGIN="$(ls $MULTUS_AUTOCONF_DIR | grep -E '\.conf(list)?$' | grep -Ev '00-multus\.conf' | head -1)"
    fi
    if [ "$MASTER_PLUGIN" == "" ]; then
      if [ $tries -lt 600 ]; then
        if ! (($tries % 5)); then
          log "Attempting to find master plugin configuration, attempt $tries"
        fi
        let "tries+=1"
        sleep 1;
      else
        error "Multus could not be configured: no master plugin was found."
        exit 1;
      fi
    else
      log "Using MASTER_PLUGIN: $MASTER_PLUGIN"

      found_master=true

      ISOLATION_STRING=""
      if [ "$MULTUS_NAMESPACE_ISOLATION" == true ]; then
        ISOLATION_STRING="\"namespaceIsolation\": true,"
      fi

      GLOBAL_NAMESPACES_STRING=""
      if [ ! -z "${MULTUS_GLOBAL_NAMESPACES// }" ]; then
        GLOBAL_NAMESPACES_STRING="\"globalNamespaces\": \"$MULTUS_GLOBAL_NAMESPACES\","
      fi

      LOG_TO_STDERR_STRING=""
      if [ "$MULTUS_LOG_TO_STDERR" == false ]; then
        LOG_TO_STDERR_STRING="\"logToStderr\": false,"
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
              error "Log levels should be one of: debug/verbose/error/panic, did not understand $MULTUS_LOG_LEVEL"
              usage
              exit 1
        esac
        LOG_LEVEL_STRING="\"logLevel\": \"$MULTUS_LOG_LEVEL\","
      fi

      LOG_FILE_STRING=""
      if [ ! -z "${MULTUS_LOG_FILE// }" ]; then
        LOG_FILE_STRING="\"logFile\": \"$MULTUS_LOG_FILE\","
      fi

      CNI_VERSION_STRING=""
      if [ ! -z "${CNI_VERSION// }" ]; then
        CNI_VERSION_STRING="\"cniVersion\": \"$CNI_VERSION\","
      fi

      ADDITIONAL_BIN_DIR_STRING=""
      if [ ! -z "${ADDITIONAL_BIN_DIR// }" ]; then
        ADDITIONAL_BIN_DIR_STRING="\"binDir\": \"$ADDITIONAL_BIN_DIR\","
      fi


      READINESS_INDICATOR_FILE_STRING=""
      if [ ! -z "${MULTUS_READINESS_INDICATOR_FILE// }" ]; then
        READINESS_INDICATOR_FILE_STRING="\"readinessindicatorfile\": \"$MULTUS_READINESS_INDICATOR_FILE\","
      fi

      if [ "$OVERRIDE_NETWORK_NAME" == "true" ]; then
        MASTER_PLUGIN_NET_NAME="$(cat $MULTUS_AUTOCONF_DIR/$MASTER_PLUGIN | \
            python -c 'import json,sys;print(json.load(sys.stdin)["name"])')"
      else
        MASTER_PLUGIN_NET_NAME="multus-cni-network"
      fi

      capabilities_python_filter_tmpfile=$(mktemp)
      cat << EOF > $capabilities_python_filter_tmpfile
import json,sys
conf = json.load(sys.stdin)
capabilities = {}
if 'plugins' in conf:
    for capa in [p['capabilities'] for p in conf['plugins'] if 'capabilities' in p]:
        capabilities.update({capability:enabled for (capability,enabled) in capa.items() if enabled})
elif 'capabilities' in conf:
    capabilities.update({capability:enabled for (capability,enabled) in conf['capabilities'] if enabled})
if len(capabilities) > 0:
    print("""\"capabilities\": """ + json.dumps(capabilities) + ",")
else:
    print("")
EOF

      NESTED_CAPABILITIES_STRING="$(cat $MULTUS_AUTOCONF_DIR/$MASTER_PLUGIN | \
            python $capabilities_python_filter_tmpfile)"
      rm $capabilities_python_filter_tmpfile
      log "Nested capabilities string: $NESTED_CAPABILITIES_STRING"

      MASTER_PLUGIN_LOCATION=$MULTUS_AUTOCONF_DIR/$MASTER_PLUGIN
      if [ "$FORCE_CNI_VERSION" == true ]; then
	      MASTER_PLUGIN_JSON="$(cat $MASTER_PLUGIN_LOCATION | sed -e "s/\"cniVersion.*/\"cniVersion\": \"$CNI_VERSION\",/g")"
      else
	      MASTER_PLUGIN_JSON="$(cat $MASTER_PLUGIN_LOCATION)"
	      log "Using $MASTER_PLUGIN_LOCATION as a source to generate the Multus configuration"
	      CHECK_CNI_VERSION=$(checkCniVersion $MASTER_PLUGIN_LOCATION $CNI_VERSION)
	      if [ "$CHECK_CNI_VERSION" != "" ] ; then
		      error "$CHECK_CNI_VERSION"
		      exit 1
	      fi
      fi

      CONF=$(cat <<-EOF
        {
          $CNI_VERSION_STRING
          "name": "$MASTER_PLUGIN_NET_NAME",
          "type": "multus",
          $NESTED_CAPABILITIES_STRING
          $ISOLATION_STRING
          $GLOBAL_NAMESPACES_STRING
          $LOG_TO_STDERR_STRING
          $LOG_LEVEL_STRING
          $LOG_FILE_STRING
          $ADDITIONAL_BIN_DIR_STRING
          $READINESS_INDICATOR_FILE_STRING
          "kubeconfig": "$MULTUS_KUBECONFIG_FILE_HOST",
          "delegates": [
            $MASTER_PLUGIN_JSON
          ]
        }
EOF
      )
      tmpfile=$(mktemp)
      echo $CONF > $tmpfile
      mv $tmpfile $CNI_CONF_DIR/00-multus.conf
      log "Config file created @ $CNI_CONF_DIR/00-multus.conf"
      echo $CONF

      # If we're not performing the cleanup on exit, we can safely rename the config file.
      if [ "$RENAME_SOURCE_CONFIG_FILE" == true ]; then
        mv ${MULTUS_AUTOCONF_DIR}/${MASTER_PLUGIN} ${MULTUS_AUTOCONF_DIR}/${MASTER_PLUGIN}.old
        log "Original master file moved to ${MULTUS_AUTOCONF_DIR}/${MASTER_PLUGIN}.old"
      fi

      if [ "$RESTART_CRIO" == true ]; then
        # Restart CRIO only once.
        if [ "$CRIO_RESTARTED_ONCE" == false ]; then
          log "Restarting crio"
          systemctl restart crio
          CRIO_RESTARTED_ONCE=true
        fi
      fi
    fi
  done
fi
}
generateMultusConf

# ---------------------- end Generate "00-multus.conf".

# Enter either sleep loop, or watch loop...
if [ "$MULTUS_CLEANUP_CONFIG_ON_EXIT" == true ]; then
  log "Entering watch loop..."
  while true; do
    # Check and see if the original master plugin configuration exists...
    if [ ! -f "$MASTER_PLUGIN_LOCATION" ]; then
      log "Master plugin @ $MASTER_PLUGIN_LOCATION has been deleted. Allowing 45 seconds for its restoration..."
      sleep 10
      for i in {1..35}
      do
        if [ -f "$MASTER_PLUGIN_LOCATION" ]; then
          log "Master plugin @ $MASTER_PLUGIN_LOCATION was restored. Regenerating given configuration."
          break
        fi
        sleep 1
      done

      generateMultusConf
      log "Continuing watch loop after configuration regeneration..."
    fi
    sleep 1
  done
else
  log "Entering sleep (success)..."
  if tty -s; then
	  read
  else
	  sleep infinity
  fi
fi
