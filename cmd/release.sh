set -ex

rm -rf ./bin/release
mkdir -p ./bin/release

make() {
    mkdir ./bin/release/daze_$1_$2
    cp ./README.md ./bin/release/daze_$1_$2/README.md
    cp ./README_CN.md ./bin/release/daze_$1_$2/README_CN.md
    cp ./res/rule.cidr ./bin/release/daze_$1_$2/rule.cidr
    cp ./res/rule.ls ./bin/release/daze_$1_$2/rule.ls
    GOOS=$1 GOARCH=$2 go build -o ./bin/release/daze_$1_$2 github.com/mohanson/daze/cmd/daze
    python -m zipfile -c ./bin/release/daze_$1_$2.zip ./bin/release/daze_$1_$2
}

# https://golang.org/doc/install/source#environment
make linux amd64
make linux arm64
make windows amd64
