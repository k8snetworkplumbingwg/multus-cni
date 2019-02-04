FROM golang:1.10-alpine as build

# Add everything
ADD . /usr/src/multus-cni

RUN apk add --update bash coreutils git && \
	cd /usr/src/multus-cni && ./build

FROM busybox:latest

ENV \
  CNI_CONF_DIR="/host/etc/cni/net.d" \
  CNI_BIN_DIR="/host/opt/cni/bin"

COPY --from=build /usr/src/multus-cni/bin/multus /opt/multus-cni/
ADD ./images/entrypoint.sh /opt/multus-cni/
ADD ./images/70-multus.conf /opt/multus-cni/

ENTRYPOINT ["/opt/multus-cni/entrypoint.sh"]
