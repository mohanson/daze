main:
	@ go run github.com/mohanson/daze/cmd/make develop

test:
	@ go test -v github.com/mohanson/daze/router
	@ go test -v github.com/mohanson/daze/protocol/ashe
