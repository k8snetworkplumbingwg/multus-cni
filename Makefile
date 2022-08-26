
IMAGE_BUILD_CMD := docker build
IMAGE_PUSH_CMD := docker push


UPSTREAM_VERSION=$(shell git describe --tags HEAD)

registry_url ?= 514845858982.dkr.ecr.us-west-1.amazonaws.com
#registry_url ?= docker.io

image_name = ${registry_url}/platform9/multus
image_tag = $(UPSTREAM_VERSION)-pmk-$(TEAMCITY_BUILD_ID)

TAG := $(image_name):$(image_tag)

SRCROOT = $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/)
BUILD_DIR :=$(SRCROOT)/bin

$(BUILD_DIR):
	mkdir -p $@

build: $(BUILD_DIR)
	$(IMAGE_BUILD_CMD) --build-arg VERSION=$(UPSTREAM_VERSION) \
		-t $(TAG) -f deployments/Dockerfile .
	echo ${TAG} > $(BUILD_DIR)/container-tag

push: build
	$(IMAGE_PUSH_CMD) $(TAG)
	&& docker rmi $(TAG)
	(docker push $(TAG)  || \
		(aws ecr get-login --region=us-west-1 --no-include-email | sh && \
		docker push $(TAG))) && \
		docker rmi $(TAG)
