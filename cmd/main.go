package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/config"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/contractmanager"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/handlers"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/repositories/contracts"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/repositories/transport"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/allocator"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/contract"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/proxy"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/system"
	"golang.org/x/sync/errgroup"
)

var (
	ErrConnectToEthNode = fmt.Errorf("cannot connect to ethereum node")
)

func main() {
	err := start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func start() error {
	var cfg config.Config
	err := config.LoadConfig(&cfg, &os.Args)
	if err != nil {
		return err
	}

	destUrl, err := url.Parse(cfg.Pool.Address)
	if err != nil {
		return err
	}

	log, err := lib.NewLogger(cfg.Log.LevelApp, cfg.Log.Color, cfg.Log.IsProd, cfg.Log.JSON, cfg.Log.FolderPath)
	if err != nil {
		return err
	}

	schedulerLog, err := lib.NewLogger(cfg.Log.LevelScheduler, cfg.Log.Color, cfg.Log.IsProd, cfg.Log.JSON, cfg.Log.FolderPath)
	if err != nil {
		return err
	}

	proxyLog, err := lib.NewLogger(cfg.Log.LevelProxy, cfg.Log.Color, cfg.Log.IsProd, cfg.Log.JSON, cfg.Log.FolderPath)
	if err != nil {
		return err
	}

	connLog, err := lib.NewLogger(cfg.Log.LevelConnection, cfg.Log.Color, cfg.Log.IsProd, cfg.Log.JSON, cfg.Log.FolderPath)
	if err != nil {
		return err
	}

	defer func() {
		_ = log.Sync()
		_ = schedulerLog.Sync()
		_ = proxyLog.Sync()
		_ = connLog.Sync()
	}()

	log.Infof("proxy-router %s", config.BuildVersion)

	if cfg.System.Enable {
		sysConfig, err := system.CreateConfigurator(log)
		if err != nil {
			return err
		}

		err = sysConfig.ApplyConfig(&system.Config{
			LocalPortRange:   cfg.System.LocalPortRange,
			TcpMaxSynBacklog: cfg.System.TcpMaxSynBacklog,
			Somaxconn:        cfg.System.Somaxconn,
			NetdevMaxBacklog: cfg.System.NetdevMaxBacklog,
			RlimitSoft:       cfg.System.RlimitSoft,
			RlimitHard:       cfg.System.RlimitHard,
		})
		if err != nil {
			log.Warnf("failed to apply system config, try using sudo or set SYS_ENABLE to false to disable\n%s", err)
			return err
		}

		defer func() {
			err = sysConfig.RestoreConfig()
			if err != nil {
				log.Warnf("failed to restore system config\n%s", err)
				return
			}
		}()
	}

	// graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-shutdownChan
		log.Warnf("Received signal: %s", s)
		cancel()

		s = <-shutdownChan
		log.Warnf("Received signal: %s. Forcing exit...", s)
		os.Exit(1)
	}()

	var (
		HashrateCounterDefault = "ema--5m"
	)

	hashrateFactory := func() *hashrate.Hashrate {
		return hashrate.NewHashrate(
			map[string]hashrate.Counter{
				HashrateCounterDefault: hashrate.NewEma(5 * time.Minute),
				"ema-10m":              hashrate.NewEma(10 * time.Minute),
				"ema-30m":              hashrate.NewEma(30 * time.Minute),
			},
		)
	}

	destFactory := func(ctx context.Context, url *url.URL, connLogID string) (*proxy.ConnDest, error) {
		return proxy.ConnectDest(ctx, url, connLog.Named(connLogID))
	}

	globalHashrate := hashrate.NewGlobalHashrate(hashrateFactory)
	alloc := allocator.NewAllocator(lib.NewCollection[*allocator.Scheduler](), log.Named("ALLOCATOR"))

	tcpServer := transport.NewTCPServer(cfg.Proxy.Address, connLog)
	tcpHandler := handlers.NewTCPHandler(
		log, connLog, proxyLog, schedulerLog,
		cfg.Miner.NotPropagateWorkerName, cfg.Miner.ShareTimeout, cfg.Miner.VettingDuration,
		destUrl,
		destFactory, hashrateFactory,
		globalHashrate, HashrateCounterDefault,
		alloc,
	)
	tcpServer.SetConnectionHandler(tcpHandler)

	ethClient, err := ethclient.DialContext(ctx, cfg.Blockchain.EthNodeAddress)
	if err != nil {
		return lib.WrapError(ErrConnectToEthNode, err)
	}

	publicUrl, err := url.Parse(cfg.Web.PublicUrl)
	if err != nil {
		return err
	}

	store := contracts.NewHashrateEthereum(common.HexToAddress(cfg.Marketplace.CloneFactoryAddress), ethClient, log)

	store.SetLegacyTx(cfg.Blockchain.EthLegacyTx)

	hrContractFactory, err := contract.NewContractFactory(
		alloc,
		hashrateFactory,
		globalHashrate,
		store,
		log,

		cfg.Marketplace.WalletPrivateKey,
		cfg.Hashrate.CycleDuration,
		cfg.Hashrate.ValidationStartTimeout,
		cfg.Hashrate.ShareTimeout,
		cfg.Hashrate.ErrorThreshold,
		cfg.Hashrate.ErrorTimeout,
	)
	if err != nil {
		return err
	}

	ownerAddr, err := lib.PrivKeyStringToAddr(cfg.Marketplace.WalletPrivateKey)
	if err != nil {
		return err
	}

	log.Infof("wallet address: %s", ownerAddr.String())

	cm := contractmanager.NewContractManager(common.HexToAddress(cfg.Marketplace.CloneFactoryAddress), ownerAddr, hrContractFactory.CreateContract, store, log)

	handl := handlers.NewHTTPHandler(alloc, cm, globalHashrate, publicUrl, HashrateCounterDefault, log)
	httpServer := transport.NewServer(cfg.Web.Address, handl, log)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return httpServer.Run(ctx)
	})

	g.Go(func() error {
		return tcpServer.Run(ctx)
	})

	g.Go(func() error {
		return cm.Run(ctx)
	})

	err = g.Wait()
	log.Infof("App exited due to %s", err)
	return err
}
