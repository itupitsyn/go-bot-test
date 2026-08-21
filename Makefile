start:
	make docker-build && make docker-run

force-start:
	make docker-build-force && make docker-run

docker-build-force:
	docker build --pull --no-cache -t go-bot-test .

docker-build:
	docker build -t go-bot-test .

docker-run:
	docker run -t -p 8080:8080 go-bot-test
