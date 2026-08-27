

.PHONY: deps-update
deps-update: ; $(info  Updating dependencies...) @ ## Update dependencies
	go mod tidy

PHONY: build test

build:
	./hack/build-go.sh

test:
	sudo ./hack/test-go.sh
	