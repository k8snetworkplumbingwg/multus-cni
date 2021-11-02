# This Dockerfile is used to build the image available on DockerHub
FROM golang:1.17.1 as build

# Add everything
ADD . /usr/src/multus-cni

RUN  cd /usr/src/multus-cni && \
     ./hack/build-go.sh

FROM centos:centos7
LABEL org.opencontainers.image.source https://github.com/k8snetworkplumbingwg/multus-cni
COPY --from=build /usr/src/multus-cni/bin /usr/src/multus-cni/bin
COPY --from=build /usr/src/multus-cni/LICENSE /usr/src/multus-cni/LICENSE
WORKDIR /

ADD ./images/entrypoint.sh /
ENTRYPOINT ["/entrypoint.sh"]
