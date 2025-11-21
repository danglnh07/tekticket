include .env.test
export $(shell sed 's/=.*//' .env.test)

test:
	go test -v ./... -coverprofile=coverage.out

run:
	docker compose down && docker compose up	

build: 
	swag init && docker compose down && docker rmi tekticket-app && docker compose up

sonar:
	docker run --rm --network=host -e SONAR_HOST_URL="http://127.0.0.1:9000" \
	-e SONAR_SCANNER_OPTS="-Dsonar.projectKey=tekticket -Dsonar.sources=. -Dsonar.go.coverage.reportPaths=coverage.out" \
    -e SONAR_TOKEN=${SONAR_TOKEN} -v "$(PWD):/usr/src" sonarsource/sonar-scanner-cli

.PHONY: test run build sonar 
