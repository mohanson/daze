if (!(Test-Path ./bin)) {
    New-Item -Path ./bin -ItemType Directory | Out-Null
}

if (!(Test-Path ./bin/delegated-apnic-latest)) {
    Invoke-WebRequest -Uri http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest -OutFile ./bin/delegated-apnic-latest | Out-Null
}

if (!(Test-Path ./bin/rule.ls)) {
    Copy-Item ./res/rule.ls -Destination ./bin/rule.ls
}

& go build -o bin github.com/mohanson/daze/cmd/daze
