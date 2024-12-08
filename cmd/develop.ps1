if (!(Test-Path bin)) {
    New-Item -Path bin -ItemType Directory | Out-Null
}

if (!(Test-Path bin/rule.cidr)) {
    Copy-Item res/rule.cidr -Destination bin/rule.cidr
}

if (!(Test-Path bin/rule.ls)) {
    Copy-Item res/rule.ls -Destination bin/rule.ls
}

& go build -o bin github.com/mohanson/daze/cmd/daze
