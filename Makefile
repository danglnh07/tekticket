include .env.test
export $(shell sed 's/=.*//' .env.test)

test:
	go test -v ./...

run:
	sudo docker compose down &&	sudo docker compose up	

build: 
	swag init && sudo docker compose down && sudo docker rmi tekticket-app && sudo docker compose up

.PHONY: test run build
