#!/usr/bin/env bash
set -xe

# This script is for the local users and developers.
# Release for github page will be maintained by github maintainers. 
# Refer .travis.yml for more info. on this.
SRC_DIR="${SRC_DIR:-$PWD}"

TAG=$(git describe --tags --dirty)
RELEASE_DIR=release-${TAG}
#Todo include  version with flag
BUILDFLAGS="-ldflags '-extldflags -static -X main.version=${TAG}'"

OUTPUT_DIR=bin

# Always clean first
rm -Rf ${SRC_DIR}/${RELEASE_DIR}
mkdir -p ${SRC_DIR}/${RELEASE_DIR}
mkdir -p ${OUTPUT_DIR}

cd ${SRC_DIR}; umask 0022;
for arch in amd64 arm arm64 ppc64le s390x; do \
    rm -f ${OUTPUT_DIR}/*; \
    CGO_ENABLED=0 GOARCH=$arch ./build.sh ${BUILDFLAGS}; \
    for format in tgz; do \
        FILENAME=multus-cni-$arch-${TAG}.$format; \
        FILEPATH=${RELEASE_DIR}/$FILENAME; \
        tar -C ${OUTPUT_DIR} --owner=0 --group=0 -caf $FILEPATH .; \
    done; \
done;
cd ${RELEASE_DIR};
  for f in *.tgz; do sha1sum $f > $f.sha1; done;
  for f in *.tgz; do sha256sum $f > $f.sha256; done;
  for f in *.tgz; do sha512sum $f > $f.sha512; done;
cd ..
chown -R ${UID} ${OUTPUT_DIR} ${RELEASE_DIR}
