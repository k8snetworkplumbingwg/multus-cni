#!/usr/bin/env bash
set -e

bash -c "umask 0; go test -v -covermode=count -coverprofile=coverage.out ./..."
