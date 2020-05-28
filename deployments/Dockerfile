# This Dockerfile is used to build the image available on DockerHub
FROM centos:centos7 as build

# Add everything
ADD . /usr/src/multus-cni

ENV INSTALL_PKGS "git golang-1.13.10-0.el7.x86_64"
RUN rpm --import https://mirror.go-repo.io/centos/RPM-GPG-KEY-GO-REPO && \
    curl -s https://mirror.go-repo.io/centos/go-repo.repo | tee /etc/yum.repos.d/go-repo.repo && \
    yum install -y $INSTALL_PKGS && \
    rpm -V $INSTALL_PKGS && \
    cd /usr/src/multus-cni && \
    ./hack/build-go.sh

FROM centos:centos7
COPY --from=build /usr/src/multus-cni /usr/src/multus-cni
WORKDIR /

ADD ./images/entrypoint.sh /
ENTRYPOINT ["/entrypoint.sh"]
