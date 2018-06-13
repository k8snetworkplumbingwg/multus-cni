#!/usr/bin/env bash
#
# Run CNI plugin tests.
# 
# This needs sudo, as we'll be creating net interfaces.
#
set -e

source ./build.sh

echo "Running tests"

GINKGO_FLAGS="-p --randomizeAllSpecs --randomizeSuites --failOnPending --progress"

# user has not provided PKG override
if [ -z "$PKG" ]; then
  GINKGO_FLAGS="$GINKGO_FLAGS -r ."
  LINT_TARGETS="./..."

# user has provided PKG override
else
  GINKGO_FLAGS="$GINKGO_FLAGS $PKG"
  LINT_TARGETS="$PKG"
fi

sudo -E bash -c "umask 0; cd ${GOPATH}/src/${REPO_PATH}; PATH=${GOROOT}/bin:$(pwd)/bin:${PATH} ginkgo ${GINKGO_FLAGS}"

cd ${GOPATH}/src/${REPO_PATH};
echo "Checking gofmt..."
fmtRes=$(go fmt $LINT_TARGETS)
if [ -n "${fmtRes}" ]; then
	echo -e "go fmt checking failed:\n${fmtRes}"
	exit 255
fi

echo "Checking govet..."
vetRes=$(go vet $LINT_TARGETS)
if [ -n "${vetRes}" ]; then
	echo -e "govet checking failed:\n${vetRes}"
	exit 255
fi
