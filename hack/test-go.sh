#!/usr/bin/env bash
set -e

# this if... will be removed when gomodules goes default
if [ "$GO111MODULE" == "off" ]; then
	echo "Warning: this will be deprecated in near future so please use go modules!"

	ORG_PATH="github.com/intel"
	REPO_PATH="${ORG_PATH}/multus-cni"

	if [ ! -h gopath/src/${REPO_PATH} ]; then
		mkdir -p gopath/src/${ORG_PATH}
		ln -s ../../../.. gopath/src/${REPO_PATH} || exit 255
	fi

	export GO15VENDOREXPERIMENT=1
	export GOBIN=${PWD}/bin
	export GOPATH=${PWD}/gopath
	bash -c "umask 0; cd ${GOPATH}/src/${REPO_PATH}; PATH=${GOROOT}/bin:$(pwd)/bin:${PATH} go test -v -covermode=count -coverprofile=coverage.out ./..."
else
	# test with go modules
	bash -c "umask 0; go test -v -covermode=count -coverprofile=coverage.out ./..."
fi
