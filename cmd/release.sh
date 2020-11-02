set -ex

rm -rf ./bin/release
mkdir ./bin/release
wget http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest -O ./bin/release/delegated-apnic-latest

make(){
    mkdir ./bin/release/daze_$1_$2
    cp ./bin/release/delegated-apnic-latest ./bin/release/daze_$1_$2/delegated-apnic-latest
    cp ./res/rule.ls ./bin/release/daze_$1_$2/rule.ls
    GOOS=$1 GOARCH=$2 go build -o ./bin/release/daze_$1_$2 github.com/mohanson/daze/cmd/daze
    python3 -m zipfile -c ./bin/release/daze_$1_$2.zip ./bin/release/daze_$1_$2
    rm -rf ./bin/release/daze_$1_$2
}

# https://golang.org/doc/install/source#environment
make linux amd64
make windows amd64

rm ./bin/release/delegated-apnic-latest
