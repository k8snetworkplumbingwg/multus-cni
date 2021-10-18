default: test

test:
	go test ./...

test-full:
	go test -v -race ./...

lint:
	go vet ./...
	@echo ""
	golint ./...

cyclo:
	-gocyclo -top 10 -avg .

report:
	@echo "misspell"
	@find . -name "*.go" | xargs misspell
	@echo ""
	-errcheck ./...
	@echo ""
	-gocyclo -over 14 -avg .
	@echo ""
	go vet ./...
	@echo ""
	golint ./...

.PHONY: test test-full lint cyclo report
