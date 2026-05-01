.PHONY: serve docker-build build install smoke-wiki

serve:
	alienshard serve

docker-build:
	docker build . -t alienshard:latest

build:
	go build .

install: build
	go install .

smoke-wiki:
	ALIEN_PORT=8001 scripts/smoke-wiki-root.sh
