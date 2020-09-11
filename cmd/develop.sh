mkdir -p bin

if [ ! -f ./bin/delegated-apnic-latest ]; then
    cd bin
    wget http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest
    cd ..
fi

if [ ! -f ./bin/rule.ls ]; then
    cp ./res/rule.ls ./bin/rule.ls
fi

go build -o bin github.com/mohanson/daze/cmd/daze
