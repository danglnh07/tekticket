include .env.test
export $(shell sed 's/=.*//' .env.test)

test:
	go test -v ./...

run:
	go run main.go

.PHONY: test run
