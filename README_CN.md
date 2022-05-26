# What's Daze?

Daze 是一款帮助你连接至**互联网**的工具.

\[[English](./README.md)]\] \[中文\]

# 使用

编译或[下载](https://github.com/mohanson/daze/releases) daze:

```sh
$ git clone https://github.com/mohanson/daze
$ cd daze

# On Linux or macOS
$ ./cmd/develop.sh
# On Windows
$ ./cmd/develop.ps1
```

构建结果将被保存在目录 `bin` 中. 只有这个目录下的文件是必须的, 你不必保留整份源代码.

使用 daze 该死的简单:

```sh
# 服务端
# 你需要一台能正确连接互联网的机器, 并输入以下命令
$ daze server -l 0.0.0.0:1081 -k $PASSWORD

# 客户端
# 使用如下命令连接至你的服务端(将 $SERVER 替换为你的服务器地址)
$ daze client -s $SERVER:1081 -k $PASSWORD
# 现在, 你即可自由地访问互联网
$ curl -x socks5://127.0.0.1:1080 google.com
```

# 在浏览器中使用, Firefox, Chrome 或 Edge

Daze 通过代理技术, 如 SOCKS4, SOCKS5 和 HTTP(S) 代理转发任何本机的 TCP/UDP 流量. 在浏览器中使用 Daze 非常简单, 以 Firefox 为例: **选项** -> **网络代理** -> **手动配置代理** -> 勾选 **SOCKS v5** 并填写 **SOCKS 主机=127.0.0.1** 和 **Port=1080**. 注意的是, 在大部分情况下, 请同时勾选底部的 **使用 SOCKS v5 时代理 DNS 查询**.

# 在 android 中使用

Daze 可以在 **Windows**, **Linux** 和 **macOS** 下正常工作. 另外, 它同样适用于 **Android**, 只是配置起来稍显复杂.

0. 交叉编译: `GOOS=linux GOARCH=arm64 go build -o daze github.com/mohanson/daze/cmd/daze`.
0. 推送二进制文件至手机. 你可以使用 `adb` 或者 `termux` + `wget`, 它们都是可行的.
0. 后台运行 `daze client -l 127.0.0.1:1080 ...`.
0. 设置代理: 连接任意 Wifi -> 设置 -> 代理 -> 填写 `127.0.0.1:1080`
0. 现在, 你即可自由地访问互联网

# 启用用户规则

Daze 使用一份名叫 RULE 的文件来管理用户自定义的过滤规则(可选的). RULE 在流量路由器中拥有最高优先级, 因此你应该小心的使用它. 这是一份合法的 RULE 文件, 并且位于 "./rule.ls". 使用 `daze client -r ./rule.ls` 来应用它.

```
L a.com
R b.com
B c.com
```

- L(ocale) 表示使用本地网络进行访问
- R(emote) 表示使用代理进行访问
- B(anned) 表示屏蔽该地址的流量, 可用于过滤广告

支持通配符, 例如 `R *.google.com`.

# 使用 CIDRs

Daze 同时使用一个 CIDR(Classless Inter-Domain Routing) 文件来进行基本的地址路由. CIDR 文件在流量路由器中优先级低于 RULE 文件, 通常位于 "./rule.cidr". 当一个 IP 地址符合 CIDR 文件中定义的规则时, daze 会使用本地网络进行访问而不是进行代理.

# 了解更多

你可以在 `daze server -h` 和 `daze client -h` 了解到所有信息.

玩的开心.
