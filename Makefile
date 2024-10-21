
IMAGE_BUILD_CMD := docker build
IMAGE_PUSH_CMD := docker push


UPSTREAM_VERSION=$(shell git describe --tags HEAD | sed 's/-.*//')

#registry_url ?= 514845858982.dkr.ecr.us-west-1.amazonaws.com
registry_url ?= quay.io

image_name = ${registry_url}/platform9/multus
image_tag = $(UPSTREAM_VERSION)-pmk-$(TEAMCITY_BUILD_ID)

TAG := $(image_name):$(image_tag)

SRCROOT = $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/)
BUILD_DIR :=$(SRCROOT)/bin
BUILD_ROOT :=$(SRCROOT)/build

$(BUILD_DIR):
	mkdir -p $@

$(BUILD_ROOT):
	mkdir -p $@
	mkdir -p $@/multus

build: $(BUILD_DIR)
	$(IMAGE_BUILD_CMD) --build-arg VERSION=$(UPSTREAM_VERSION) \
		-t $(TAG) -f deployments/Dockerfile .
	echo ${TAG} > $(BUILD_DIR)/container-tag

scan: $(BUILD_ROOT)
	docker run -v $(BUILD_ROOT)/multus:/out -v /var/run/docker.sock:/var/run/docker.sock  aquasec/trivy image -s CRITICAL,HIGH -f json  --vuln-type library -o /out/library_vulnerabilities.json --exit-code 22 ${TAG}
	docker run -v $(BUILD_ROOT)/multus:/out -v /var/run/docker.sock:/var/run/docker.sock  aquasec/trivy image -s CRITICAL,HIGH -f json  --vuln-type os -o /out/os_vulnerabilities.json --exit-code 0 ${TAG}

push: 
	docker login
	$(IMAGE_PUSH_CMD) $(TAG) \
	&& docker rmi $(TAG)
	
