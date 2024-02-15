#!/bin/sh
#set -o errexit

export PATH=${PATH}:./bin

# delete cluster kind
kind delete cluster
