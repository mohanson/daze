# What's Daze?

Daze is a tool to help you link to the **Internet**.

# Usage

```sh
go get -u -v github.com/mohanson/daze/cmd/daze

# server port
# you need a linux machine that can access the Internet, and enter the following command:
daze server -l 0.0.0.0:51958

# client port
# use the following command to link your server:
daze client -s $SERVER:51958 -l 127.0.0.1:51959
# now, you are free to visit Internet
daze cmd curl https://google.com
```
