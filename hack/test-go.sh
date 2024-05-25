#!/usr/bin/env bash
set -e

bash -c "umask 0; go test -v -race -covermode=atomic -coverprofile=coverage.out ./..."
