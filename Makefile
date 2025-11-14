include .env.test
export $(shell sed 's/=.*//' .env.test)

test:
	go test -v ./... -coverprofile=coverage.out

run:
	sudo docker compose down &&	sudo docker compose up	

build: 
	swag init && sudo docker compose down && sudo docker rmi tekticket-app && sudo docker compose up

sonar:
	sudo docker run --rm --network=host -e SONAR_HOST_URL="http://127.0.0.1:9000" \
	-e SONAR_SCANNER_OPTS="-Dsonar.projectKey=tekticket -Dsonar.sources=. -Dsonar.go.coverage.reportPaths=coverage.out" \
    -e SONAR_TOKEN=${SONAR_TOKEN} -v "$(PWD):/usr/src" sonarsource/sonar-scanner-cli

.PHONY: test run build
