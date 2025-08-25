# Pretty

Package pretty provides utilities for beautifying console output.

**Progress**

```sh
$ go run cmd/progress/main.go

2025/03/12 09:53:42 pretty: [=========================>                   ]  59%
```

**Table**

```sh
$ go run cmd/table/main.go

2025/03/12 09:51:57 pretty: City name Area Population Annual Rainfall
2025/03/12 09:51:57 pretty: -----------------------------------------
2025/03/12 09:51:57 pretty:  Adelaide 1295    1158259           600.5
2025/03/12 09:51:57 pretty:  Brisbane 5905    1857594          1146.4
2025/03/12 09:51:57 pretty:    Darwin  112     120900          1714.7
2025/03/12 09:51:57 pretty:    Hobart 1357     205556           619.5
2025/03/12 09:51:57 pretty: Melbourne 1566    3806092           646.9
2025/03/12 09:51:57 pretty:     Perth 5386    1554769           869.4
2025/03/12 09:51:57 pretty:    Sydney 2058    4336374          1214.8
```
