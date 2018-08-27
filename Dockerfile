FROM centos:centos7

# Add everything
ADD . /usr/src/multus-cni

ENV INSTALL_PKGS "git golang"
RUN yum install -y $INSTALL_PKGS && \
    rpm -V $INSTALL_PKGS && \
    cd /usr/src/multus-cni && \
    ./build && \
    yum autoremove -y $INSTALL_PKGS && \
    yum clean all && \
    rm -rf /tmp/*

WORKDIR /

LABEL io.k8s.display-name="Multus CNI" \
      io.k8s.description="This is a component of OpenShift Container Platform and provides a meta CNI plugin." \
      io.openshift.tags="openshift" \
      maintainer="Doug Smith <dosmith@redhat.com>"

ADD ./images/entrypoint.sh /

# does it require a root user?
# USER 1001

ENTRYPOINT /entrypoint.sh
