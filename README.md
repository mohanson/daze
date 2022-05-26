# What's Daze?

Daze is a tool to help you link to the **Internet**.

\[English\] \[[中文](./README_CN.md)\]

# Usage

Compile or [Download](https://github.com/mohanson/daze/releases) daze:

```sh
$ git clone https://github.com/mohanson/daze
$ cd daze

# On Linux or macOS
$ ./cmd/develop.sh
# On Windows
$ ./cmd/develop.ps1
```

Build results will be saved in directory `bin`. You can just keep this directory, all other files are not required.

Daze is dead simple to use:

```sh
# server port
# you need a machine that can access the Internet, and enter the following command:
$ daze server -l 0.0.0.0:1081 -k $PASSWORD

# client port
# use the following command to link your server(replace $SERVER with your server ip):
$ daze client -s $SERVER:1081 -k $PASSWORD
# now, you are free to visit Internet
$ curl -x socks5://127.0.0.1:1080 google.com
```

# For browser, Firefox, Chrome or Edge e.g.

Daze forces any TCP/UDP connection to follow through proxy like SOCKS4, SOCKS5 or HTTP(S) proxy. It can be simply used in browser, take Firefox as an example: Open `Connection Settings` -> `Manual proxy configuration` -> `SOCKSv5 Host=127.0.0.1` and `Port=1080`.

# For android

Daze can work well on **Windows**, **Linux** and **macOS**. In additional, it can also work on **Android**, just it will be a bit complicated.

0. Cross compile daze for android: `GOOS=linux GOARCH=arm64 go build -o daze github.com/mohanson/daze/cmd/daze`
0. Push the compiled file to the phone. You can use `adb` or `termux` + `wget`, they are both possible.
0. Run `daze client -l 127.0.0.1:1080 ...` in the background.
0. Set the proxy for phone: WLAN -> Settings -> Proxy -> Fill in `127.0.0.1:1080`
0. Now, you are free to visit Internet.

# Use custom rules

Daze use a RULE file to custom your own rules(optional). RULE has the highest priority in routers, so that you should carefully maintain it. This is a RULE document located at "./rule.ls", use `daze client -r ./rule.ls` to apply it.

```
L a.com
R b.com
B c.com
```

- L(ocale) means using local network
- R(emote) means using proxy
- B(anned) means block it, often used to block ads

Glob is supported, such as `R *.google.com`.

# Use CIDRs

Daze also use a CIDR(Classless Inter-Domain Routing) file to routing addresses. CIDR file has lower priority than RULE files, located at "./rule.cidr". When a IP address is in the CIDR file, daze will use the local network to establish the connection instead of using a proxy.

# More

You can find all the information here by using `daze server -h` and `daze client -h`.

Have fun.
