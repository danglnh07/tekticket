test:
	go test -v -cover ./...

run:
	go run main.go

.PHONY: test run
