FROM busybox:latest

ENV \
  CNI_CONF_DIR="/host/etc/cni/net.d" \
  CNI_BIN_DIR="/host/opt/cni/bin"


ADD bin/multus /opt/multus-cni/
ADD ./images/entrypoint.sh /opt/multus-cni/
ADD ./images/70-multus.conf /opt/multus-cni/

ENTRYPOINT ["/opt/multus-cni/entrypoint.sh"]
