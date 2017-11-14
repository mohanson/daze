```sh
go get -u -v github.com/mohanson/daze/cmd/daze

# server port
daze server -b 0.0.0.0:51958

# client port
daze client -s $SERVER:51958 -b 127.0.0.1:51959
```
