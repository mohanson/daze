```sh
go get -u -v github.com/mohanson/daze/cmd/daze

# server port
daze server -l 0.0.0.0:51958

# client port
daze client -s $SERVER:51958 -l 127.0.0.1:51959
```
