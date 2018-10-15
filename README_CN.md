# What's Daze?

Daze 是一款帮助你连接至**互联网**的工具.

\[[English](./README.md)]\] \[中文\]

# 使用

编译或[下载](https://github.com/mohanson/daze/releases) daze:

```sh
$ go get -u -v github.com/mohanson/daze/cmd/daze
```

使用 daze 该死的简单:

```sh
# 服务端
# 你需要一台能正确连接互联网的机器, 并输入以下命令
$ daze server -l 0.0.0.0:51958

# 客户端
# 使用如下命令连接至你的服务端(将 $SERVER 替换为你的服务器地址)
$ daze client -s $SERVER:51958 -l 127.0.0.1:51959 -dns 114.114.114.114:53
# 现在, 你即可自由地访问互联网
$ daze cmd curl https://google.com
```

# 在浏览器中使用, Firefox, Chrome 或 Edge

Daze 通过代理技术, 如 SOCKS4, SOCKS5 和 HTTP(S) 代理转发任何本机的 TCP/UDP 流量. 在浏览器中使用 Daze 非常简单, 以 Firefox 为例: **选项** -> **网络代理** -> **手动配置代理** -> 勾选 **SOCKS v5** 并填写 **SOCKS 主机=127.0.0.1** 和 **Port=51959**. 注意的是, 在大部分情况下, 请同时勾选底部的 **使用 SOCKS v5 时代理 DNS 查询**.

# 在 android 中使用

Daze 可以在 **Windows**, **Linux** 和 **macOS** 下正常工作. 另外, 它同样适用于 **Android**, 只是配置起来稍显复杂.

1. 下载 [SDK Platform Tools](https://developer.android.com/studio/releases/platform-tools) 并确保你能正常使用 `adb` 命令.
2. 使用 USB 连接你的手机和电脑. 使用 `adb devices` 可显示已连接的设备, 确保连接成功.
2. 交叉编译: `GOOS=linux GOARCH=arm go build -o daze github.com/mohanson/daze/cmd/daze`
4. 推送二进制文件至手机并进入 Shell: `adb push daze /data/local/tmp/daze`, `adb shell`
5. 启动 daze 客户端: `cd /data/local/tmp`, `chmod +x daze`, `daze client -s $SERVER:51958 -l 127.0.0.1:51959 -dns 114.114.114.114:53`. 注意的是, 你可能需要使用 `setsid` 命令将客户端程序托管至后台运行.
6. 设置代理: 连接任意 Wifi -> 设置 -> 代理 -> 填写 `127.0.0.1:51959`
7. 现在, 你即可自由地访问互联网

# 了解更多

你可以在 `daze server -h` 和 `daze client -h` 了解到所有信息. Cli 提供了如下可配置项目

- 数据加密
- 混淆
- 指定 DNS
- 选择流量过滤模式: 自动, 无或仅过滤中国 IP(默认)

玩的开心.
