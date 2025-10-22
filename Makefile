include .env.test
export $(shell sed 's/=.*//' .env.test)

test:
	go test -v ./...

run:
	go run main.go

build: 
	sudo docker compose down && sudo docker rmi tekticket && sudo docker compose up

.PHONY: test run
