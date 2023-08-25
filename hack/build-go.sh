#!/usr/bin/env bash
set -e

DEST_DIR="bin"

if [ ! -d ${DEST_DIR} ]; then
	mkdir ${DEST_DIR}
fi

# Specify correspondingGOARCH from TARGETPLATFORM
if [ "$TARGETPLATFORM" = "linux/amd64" ]; then
	export GOARCH=amd64
elif [ "$TARGETPLATFORM" = "linux/arm64" ]; then
	export GOARCH=arm64
elif [ "$TARGETPLATFORM" = "linux/arm/v7" ]; then
	export GOARCH=arm
elif [ "$TARGETPLATFORM" = "linux/ppc64le" ]; then
	export GOARCH=ppc64le
elif [ "$TARGETPLATFORM" = "linux/s390x" ]; then
	export GOARCH=s390x
fi

# version information
hasGit=true
git version > /dev/null 2>&1 || hasGit=false
GIT_SHA=""
GIT_TREE_STATE=""
GIT_TAG=""
GIT_TAG_LAST=""
RELEASE_STATUS=""
if $hasGit; then
	set +e
	GIT_SHA=$(git rev-parse --short HEAD)
	# Tree state is "dirty" if there are uncommitted changes, untracked files are ignored
	GIT_TREE_STATE=$(test -n "`git status --porcelain --untracked-files=no`" && echo "dirty" || echo "clean")
	# Empty string if we are not building a tag
	GIT_TAG=$(git describe --tags --abbrev=0 --exact-match 2>/dev/null || true)
	# Find most recent tag
	GIT_TAG_LAST=$(git describe --tags --abbrev=0 2>/dev/null || true)
	set -e
fi

# VERSION override mechanism if needed
VERSION=${VERSION:-}
if [[ -n "${VERSION}" || -n "${GIT_TAG}" ]]; then
	RELEASE_STATUS=",released"
fi

if [ -z "$VERSION" ]; then
	VERSION=$GIT_TAG_LAST
fi
# Add version/commit/date into binary
DATE=$(date -u -d "@${SOURCE_DATE_EPOCH:-$(date +%s)}" --iso-8601=seconds)
COMMIT=${COMMIT:-$(git rev-parse --verify HEAD)}
LDFLAGS="-X gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/multus.version=${VERSION} \
	-X gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/multus.commit=${COMMIT} \
	-X gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/multus.gitTreeState=${GIT_TREE_STATE} \
	-X gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/multus.releaseStatus=${RELEASE_STATUS} \
	-X gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/multus.date=${DATE}"
export CGO_ENABLED=${CGO_ENABLED:-0}

# build with go modules
export GO111MODULE=on

if [ -n "$MODMODE" ]; then
	BUILD_ARGS=(-mod "$MODMODE")
fi

echo "Building multus"
go build -o ${DEST_DIR}/multus ${BUILD_ARGS} -ldflags "${LDFLAGS}" "$@" ./cmd/multus
echo "Building multus-daemon"
go build -o "${DEST_DIR}"/multus-daemon ${BUILD_ARGS} -ldflags "${LDFLAGS}" ./cmd/multus-daemon
echo "Building multus-shim"
go build -o "${DEST_DIR}"/multus-shim ${BUILD_ARGS} -ldflags "${LDFLAGS}" ./cmd/multus-shim
echo "Building install_multus"
go build -o "${DEST_DIR}"/install_multus ${BUILD_ARGS} -ldflags "${LDFLAGS}" ./cmd/install_multus
echo "Building thin_entrypoint"
go build -o "${DEST_DIR}"/thin_entrypoint ${BUILD_ARGS} -ldflags "${LDFLAGS}" ./cmd/thin_entrypoint
