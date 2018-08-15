# What's Daze?

Daze is a tool to help you link to the **Internet**.

# Usage

Daze is dead simple to use:

```sh
$ go get -u -v github.com/mohanson/daze/cmd/daze

# server port
# you need a linux machine that can access the Internet, and enter the following command:
$ daze server -l 0.0.0.0:51958

# client port
# use the following command to link your server:
$ daze client -s $SERVER:51958 -l 127.0.0.1:51959 -dns 114.114.114.114:53
# now, you are free to visit Internet
$ daze cmd curl https://google.com
```

# For browser, Firefox, Chrome or Edge e.g.

Daze forces any TCP connection to follow through proxy like SOCKS4, SOCKS5 or HTTP(S) proxy. It can be simply used in browser, take Firefox as an example: Open `Connection Settings` -> `Manual proxy configuration` -> `SOCKS Host=127.0.0.1` and `Port=51959`.

# For android

Daze can work well on **Windows**, **Linux** and **macOS**. In additional, it can also work on **Android**, just it will be a bit complicated.

1. Download [SDK Platform Tools](https://developer.android.com/studio/releases/platform-tools) and make sure you can use `adb` normally.
2. Connect your phone to your computer with USB. Use `adb devices` to list devices.
2. Cross compile daze for android: `GOOS=linux GOARCH=arm go build -o daze github.com/mohanson/daze/cmd/daze`
4. Push binary and open shell: `adb push daze /data/local/tmp/daze`, `adb shell`
5. Open daze client: `cd /data/local/tmp`, `chmod +x daze`, `daze client -s $SERVER:51958 -l 127.0.0.1:51959 -dns 114.114.114.114:53`. Attention, you may wish use `setsid` to run daze in a new session.
6. Set the proxy for phone: WLAN -> Settings -> Proxy -> Fill in `127.0.0.1:51959`
7. Now, you are free to visit Internet.

# For python

`daze cmd` can proxy most common applications likes `curl` or `wget`(which all used `libcurl`). You can easily apply daze in your own python code also. All you need is just install `pysocks` and run codes with `daze cmd`.

```sh
$ pip install pysocks requests
```

Write the following codes to a file named "google.py":

```py
import requests
r = requests.get('https://google.com')
print(r.status_code)
```

Use `daze cmd python google.py` instead of `python google.py`

```sh
$ daze cmd python google.py
# 200
```
