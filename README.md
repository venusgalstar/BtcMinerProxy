# EtherProxy for NiceHash and Ethereum mining pools

This is an Ethereum mining proxy with simple web interface.

- [Features](#features)
- [Building on Linux](#buildingonlinux)
- [Building on Windows](#buildingonwindows)
- [Building on Mac OS X](#buildingonmacosx)
- [Configuration](#configuration)
- [Example upstream section](#exampleupstream)
- [Running](#running)
- [Connecting and mining with ethminer to the proxy](#connectingandmining)
- [JSON API stats](#jsonapistats)
- [Pools that work with this proxy](#supportedpools)
- [TODO](#todo)
- [Acknowledgements](#acknowledgements)
- [License](#license)

![Demo](https://raw.githubusercontent.com/sammy007/ether-proxy/master/proxy.png)

### <a name="features"></a> Features

* Increases efficiency and profitability by aggregating work from several miners on a single connection to the pool.
* Simple web interface with statistics.
* Rigs availability monitoring.
* Keep track of accepts, rejects, blocks stats.
* Easy detection of sick rigs.
* Daemon failover list.
* JSON stats output.

### <a name="buildingonlinux"></a> Building on Linux

Dependencies:

* go >= 1.4
* geth

Export GOPATH:

export GOPATH=$HOME/go

Install required packages:

go get github.com/ethereum/ethash
go get github.com/ethereum/go-ethereum/common
go get github.com/goji/httpauth
go get github.com/gorilla/mux
go get github.com/yvasiyarov/gorelic

Compile:

go build -o ether-proxy main.go

### <a name="buildingonwindows"></a> Building on Windows

Follow [this wiki paragraph](https://github.com/ethereum/go-ethereum/wiki/Installation-instructions-for-Windows#building-from-source) in order to prepare your environment.
Install required packages (look at Linux install guide above). Then compile:

go build -o ether-proxy.exe main.go

### <a name="buildingonmacosx"></a> Building on Mac OS X

If you didn't install [Brew](http://brew.sh/), do it. Then install Golang:

brew install go

And follow Linux installation instructions because they are the same for OS X.

### <a name="configuration"></a> Configuration

Configuration is self-describing, just copy *config.example.json* to *config.json* and specify endpoint URL and upstream URLs.

#### <a name="exampleupstream"></a> Example upstream section

```javascript
"upstream": [
{
"pool": true,
"name": "NiceHash as primary pool",
"url": "http://ethereum.eu.nicehash.com:3500/n1c3-1CzrFvvieNaZg5aMHkb8eAPCSeVVUfUpax.myproxy/250",
"timeout": "10s"
},
{
"pool": true,
"name": "suprnova.cc as secondary pool",
"url": "http://eth-mine.suprnova.cc:3000/myusername.myproxy/250",
"timeout": "10s"
},
{
"name": "local geth wallet as backup",
"url": "http://127.0.0.1:8545",
"timeout": "10s"
}
],
```

In this example we specified [NiceHash](https://www.nicehash.com) mining pool as main primary mining target. Additionally we specified [suprnova.cc](https://eth.suprnova.cc) and a local geth node as backup.

With <code>"submitHashrate": true|false</code> proxy will forward <code>eth_submitHashrate</code> requests to upstream.

#### <a name="running"></a> Running

./ether-proxy config.json

#### <a name="connectingandmining"></a> Connecting and mining with ethminer to the proxy

The syntax is:

ethminer -G -F http://x.x.x.x:8546/miner/N/X

where N = desired difficulty (approximately expected rig hashrate, divided by 2) and X is the name of the rig.

Examples for one AMD and one NVIDIA rig with multiple GPUs:

ethminer -G -F http://x.x.x.x:8546/miner/50/myamdrig
ethminer -U -F http://x.x.x.x:8546/miner/30/mynvidiarig

### <a name="jsonapistats"></a> JSON API stats

Point your browser to the stats page according to "frontend" settings in config.json:

http://x.x.x.x:8080/stats

### <a name="supportedpools"></a> Pools that work with this proxy

* [NiceHash](https://www.nicehash.com) Ethereum pool with auto-convert to Bitcoin
* [suprnova.cc](https://eth.suprnova.cc) Ethereum pool

### <a name="todo"></a> TODO

* Report block numbers
* Report luck per rig
* Maybe add more stats
* Maybe add charts

### <a name="acknowledgements"></a> Acknowledgements

Forked from [sammy007's ethere-proxy](https://github.com/sammy007/ether-proxy), all credits goes to sammy007.

### <a name="license"></a> License

The MIT License (MIT).
