VERSION=$(grep '^VERSION=' .version | cut -d '=' -f 2-)
echo VERSION=$VERSION
go build -ldflags="-s -w -X 'gitlab.com/TitanInd/proxy/proxy-router-v3/internal/config.BuildVersion=$VERSION'" -o bin/hashrouter cmd/main.go 
