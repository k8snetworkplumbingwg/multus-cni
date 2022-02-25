
IMAGE_BUILD_CMD := docker build
IMAGE_PUSH_CMD := docker push
VERSION ?= v3.7.2
BUILD_NUMBER ?= 1

IMAGE_REGISTRY := docker.io/platform9
IMAGE_NAME := multus
IMAGE_TAG_NAME := $(VERSION)-pmk-$(BUILD_NUMBER)
IMAGE_REPO := $(IMAGE_REGISTRY)/$(IMAGE_NAME)
IMAGE_TAG := $(IMAGE_REPO):$(IMAGE_TAG_NAME)

SRCROOT = $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/)
BUILD_DIR :=$(SRCROOT)/bin

$(BUILD_DIR):
	mkdir -p $@

build: $(BUILD_DIR)
	$(IMAGE_BUILD_CMD) --build-arg VERSION=$(VERSION) \
		-t $(IMAGE_TAG) -f deployments/Dockerfile .
	echo ${IMAGE_TAG} > $(BUILD_DIR)/container-tag

push: build
	$(IMAGE_PUSH_CMD) $(IMAGE_TAG)