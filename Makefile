build: # сборка контейнера и запуск api
	docker compose up --build -d

lint: # проверка кода линтером golangci-lint
	golangci-lint run
	
test: # запуск тестов
	go test -v ./...