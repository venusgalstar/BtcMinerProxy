go mod tidy
rm -r ./build
mkdir ./build
cd ./build

echo Building for Linux...
export CGO_ENABLED="0"
export GOOS="linux"
export GOARCH="amd64"
go build ../
mv ./kiloproxy ./kiloproxy-linux-x64
xz -9 -e ./kiloproxy-linux-x64

rm ./kiloproxy.exe

echo Done.

sha256sum *