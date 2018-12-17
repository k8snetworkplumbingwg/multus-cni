FROM busybox:latest

ADD bin/multus /opt/multus-cni/
ADD ./images/entrypoint.sh /opt/multus-cni/

ENTRYPOINT ["/opt/multus-cni/entrypoint.sh"]
