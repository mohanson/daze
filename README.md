<div align="center" style="background-color: #FFFFFF"><img src="./res/daze.png"></div>

Daze is software to help you pass through the firewalls, in other words, proxy. Daze uses a simple but efficient protocol, making sure you never get detected or blocked.

# Tutorials

Daze is designed as a single-file application. First of all, compile or [download](https://github.com/mohanson/daze/releases) daze:

```sh
$ git clone https://github.com/mohanson/daze
$ cd daze

# On Linux or macOS
$ ./cmd/develop.sh
# On Windows
$ ./cmd/develop.ps1
```

Build results will be saved in the directory `bin`. You can just keep this directory, and all other files are not required.

Daze is dead simple to use:

```sh
# Server side
# You need a machine that can access the Internet, and enter the following command:
$ daze server -l 0.0.0.0:1081 -k $PASSWORD

# Client side
# Use the following command to link your server(replace $SERVER with your server IP):
$ daze client -s $SERVER:1081 -k $PASSWORD
# Now, you are free to visit the Internet
$ curl -x socks5://127.0.0.1:1080 google.com
```

# Guides

For users who have already finished tutorials, here is an advanced guide.

## Platforms

Daze is implemented in pure Go language, so it can run on almost any operating system. The following are only the browsers/operating system commonly used by me:

### Android

0. Cross compile daze for android: `GOOS=android GOARCH=arm64 go build -o daze github.com/mohanson/daze/cmd/daze`
0. Push the compiled file to the phone. You can use [adb](https://developer.android.com/studio/command-line/adb) or create an HTTP server and download daze with `wget` in [termux](https://play.google.com/store/apps/details?id=com.termux&hl=en).
0. Run `daze client -l 127.0.0.1:1080 ...` in the termux.
0. Set the proxy for phone: WLAN -> Settings -> Proxy -> Fill in `127.0.0.1:1080`

### Chrome

Chrome does not support setting proxies, so a third-party plugin must be used. [Proxy SwitchyOmega](https://chrome.google.com/webstore/detail/proxy-switchyomega/padekgcemlokbadohgkifijomclgjgif?hl=en) works very well.

### Firefox

Firefox can configure a proxy in `Connection Settings` -> `Manual proxy configuration` -> `SOCKSv5 Host=127.0.0.1` and `Port=1080`. If you see an option `Use remote DNS` on the page, check it boldly.

## Network Model And Concepts

Daze's network model consists of 5 characters:

```text
+-------------+        +-------------+        +----------+        +-------------+        +-----------+
| Destination | <----> | Daze server | <----> | Firewall | <----> | Daze client | <----> |    User   |
+-------------+        +------+------+        +----------+        +-------------+        +-----------+
                              |                                          |                     |
                              +------------- Middle Protocol ------------+-- Client Protocol --+
```

- Destination: Internet service provider. For example, google.com.
- Daze Server: a daze instance run by the command `daze server`.
- Firewall: a firewall is a network security system that monitors and controls the incoming and outgoing network traffic based on predetermined security rules.
- Daze client: a daze instance run by the command `daze client`.
- User: a browser or other application trying to access the dest.
- Middle Protocol: communication protocol between the daze server and daze client. Data is encrypted and obfuscated to bypass firewalls.
- Client Protocol: communication protocol between the daze client and user.

## Protocols

### Client Protocols

Daze client implements 5 different proxy protocols in one port, they are HTTP Proxy, HTTPS Tunnel, SOCKS4, SOCKS4a, and SOCKS5.

```sh
# HTTP Proxy
$ curl -x http://127.0.0.1:1080    http(s)://google.com
# HTTPS Tunnel
$ curl -x http://127.0.0.1:1080    http(s)://google.com
# SOCKS4
$ curl -x socks4://127.0.0.1:1080  http(s)://google.com
# SOCKS4a
$ curl -x socks4a://127.0.0.1:1080 http(s)://google.com
# SOCKS5
$ curl -x socks5://127.0.0.1:1080  http(s)://google.com
```

Why can one port support so many protocols? Because it's magic!

### Middle Protocols

Daze currently has 4 middle protocols.

#### Ashe

Default protocol. Ashe is a TCP-based cryptographic proxy protocol. The main purpose of this protocol is to bypass firewalls while providing a good user experience, so it only provides minimal security, which is one of the reasons for choosing the RC4 algorithm.

Note that it is your responsibility to ensure that the server and client's date and time are consistent, the ashe protocol allows a deviation of two minutes.

#### Baboon

Protocol baboon is the ashe protocol based on HTTP. The daze server will pretend to be an HTTP service. If the user sends the correct password, the daze server will provide the proxy service, otherwise, it will behave as a normal HTTP service. To use the baboon protocol, you need to specify the protocol name and a fake site:

```sh
$ daze server ... -p baboon -e https://github.com
$ daze client ... -p baboon
```

#### Czar

Protocol czar is the ashe protocol base on TCP multiplexing. For the uninitiated, multiplexing is the practice of reusing a single TCP connection for multiple ashe protocols. This practice saves the time of the TCP three-way handshake, but on the other hand, there is a small impairment to the data transfer rate (about 0.19%). In most cases, it has a better user experience than using the ashe protocol directly.

```sh
$ daze server ... -p czar
$ daze client ... -p czar
```

#### Dahlia

Dahlia is an encrypted port forwarding protocol. Unlike common port forwarding tools, it needs to configure a server and a client, and the communication between the server and the client is encrypted to bypass firewall detection.

```sh
# Port forwarding from 20002 to 20000:
$ daze server -l :20001 -e 127.0.0.1:20000 -p dahlia
$ daze client -l :20002 -s 127.0.0.1:20001 -p dahlia
```

## Proxy Control

Proxy control is a rule that determines whether network requests (TCP and UDP) go directly to the destination or are forwarded to the daze server. Use the `-f` option in the daze client to adjust the proxy configuration, you can choose

- Use local network for all requests.
- Use remote server for all requests.
- Use both local and remote server (default).

### File rule.ls

Daze uses a "rule.ls" file to customize your own rules(optional). "rule.ls" has the highest priority in routers so you should carefully maintain it. The "rule.ls" is located on the "./rule.ls" by default, or you can use `daze client -r path/to/rule.ls` to apply it.

```
L a.com
R b.com
B c.com
```

- L(ocale) means using local network
- R(emote) means using proxy
- B(anned) means to block it, often used to block ads

Glob is supported, such as `R *.google.com`.

### File rule.cidr

Daze also uses a CIDR(Classless Inter-Domain Routing) file to route addresses. The CIDR file is located at "./rule.cidr", and has a lower priority than "rule.ls".

By default, daze has configured rule.cidr for China's mainland.
