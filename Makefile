PHONY: build test

build:
	./hack/build-go.sh

test:
	sudo ./hack/test-go.sh