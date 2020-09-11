main:
	@ go run cmd/make/main.go develop

test:
	@ go test -v github.com/mohanson/daze/router
	@ go test -v github.com/mohanson/daze/protocol/ashe
