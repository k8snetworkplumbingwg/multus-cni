#!/usr/bin/env bash
set -e

export GO111MODULE=on

bash -c "umask 0; go test -v -covermode=count -coverprofile=coverage.out ./..."

