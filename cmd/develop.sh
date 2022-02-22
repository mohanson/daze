set -ex

if [ ! -d ./bin ]; then
    mkdir bin
fi

if [ ! -f ./bin/rule.cidr ]; then
    cp ./res/rule.cidr ./bin/rule.cidr
fi

if [ ! -f ./bin/rule.ls ]; then
    cp ./res/rule.ls ./bin/rule.ls
fi

go build -o bin github.com/mohanson/daze/cmd/daze
