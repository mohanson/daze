go run cmd/make/main.go develop

ashe reload ezad_server
ashe reload ezad_client
echo done

ashe init -n ezad_server -- ./bin/daze server -l 127.0.0.1:7777
ashe init -n ezad_client -- ./bin/daze client -s 127.0.0.1:7777 -f none

go run cmd/make/main.go develop && ashe reload ezad_server && ashe reload ezad_client
