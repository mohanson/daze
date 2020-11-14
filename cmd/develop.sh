set -ex

if [ ! -d ./bin ]; then
    mkdir bin
fi

if [ ! -f ./bin/delegated-apnic-latest ]; then
    wget -q http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest -O ./bin/delegated-apnic-latest
fi

if [ ! -f ./bin/rule.ls ]; then
    cp ./res/rule.ls ./bin/rule.ls
fi

go build -o bin github.com/mohanson/daze/cmd/daze
