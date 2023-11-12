package contracts

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/Lumerin-protocol/contracts-go/clonefactory"
	"github.com/Lumerin-protocol/contracts-go/implementation"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
	hr "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
)

const (
	SUBSCRIPTION_MAX_RECONNECTS = 50 // max consequent reconnects
)

type HashrateEthereum struct {
	// config
	legacyTx bool // use legacy transaction fee, for local node testing

	// state
	nonce   uint64
	mutex   sync.Mutex
	cfABI   *abi.ABI
	implABI *abi.ABI

	// deps
	cloneFactory *clonefactory.Clonefactory
	client       EthereumClient
	log          interfaces.ILogger
}

func NewHashrateEthereum(clonefactoryAddr common.Address, client EthereumClient, log interfaces.ILogger) *HashrateEthereum {
	cf, err := clonefactory.NewClonefactory(clonefactoryAddr, client)
	if err != nil {
		panic("invalid clonefactory ABI")
	}
	cfABI, err := clonefactory.ClonefactoryMetaData.GetAbi()
	if err != nil {
		panic("invalid clonefactory ABI: " + err.Error())
	}
	implABI, err := implementation.ImplementationMetaData.GetAbi()
	if err != nil {
		panic("invalid implementation ABI: " + err.Error())
	}
	return &HashrateEthereum{
		cloneFactory: cf,
		client:       client,
		cfABI:        cfABI,
		implABI:      implABI,
		log:          log,
	}
}

func (g *HashrateEthereum) SetLegacyTx(legacyTx bool) {
	g.legacyTx = legacyTx
}

func (g *HashrateEthereum) GetContractsIDs(ctx context.Context) ([]string, error) {
	hashrateContractAddresses, err := g.cloneFactory.GetContractList(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, err
	}

	addresses := make([]string, len(hashrateContractAddresses))
	for i, address := range hashrateContractAddresses {
		addresses[i] = address.Hex()
	}

	return addresses, nil
}

func (g *HashrateEthereum) GetContract(ctx context.Context, contractID string) (*hashrate.EncryptedTerms, error) {
	instance, err := implementation.NewImplementation(common.HexToAddress(contractID), g.client)
	if err != nil {
		return nil, err
	}

	data, err := instance.GetPublicVariables(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, err
	}

	terms := &hashrate.EncryptedTerms{
		Base: hashrate.Base{
			ContractID: contractID,
			Seller:     data.Seller.Hex(),
			Duration:   time.Duration(data.Length.Int64()) * time.Second,
			Hashrate:   float64(hr.HSToGHS(float64(data.Speed.Int64()))),
			State:      hashrate.BlockchainState(data.State),
		},
	}

	if data.State == 1 { // running
		startsAt := time.Unix(data.StartingBlockTimestamp.Int64(), 0)
		terms.StartsAt = &startsAt
		terms.Buyer = data.Buyer.Hex()
		terms.DestEncrypted = data.EncryptedPoolData
	}

	return terms, nil
}

func (g *HashrateEthereum) PurchaseContract(ctx context.Context, contractID string, privKey string) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	opts, err := g.getTransactOpts(ctx, privKey)
	if err != nil {
		return err
	}

	_, err = g.cloneFactory.SetPurchaseRentalContract(opts, common.HexToAddress(contractID), "")
	if err != nil {
		g.log.Error(err)
		return err
	}

	watchOpts := &bind.WatchOpts{
		Context: ctx,
	}
	sink := make(chan *clonefactory.ClonefactoryClonefactoryContractPurchased)
	sub, err := g.cloneFactory.WatchClonefactoryContractPurchased(watchOpts, sink, []common.Address{})
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	select {
	case <-sink:
		return nil
	case err := <-sub.Err():
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

}

func (g *HashrateEthereum) CloseContract(ctx context.Context, contractID string, closeoutType CloseoutType, privKey string) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	instance, err := implementation.NewImplementation(common.HexToAddress(contractID), g.client)
	if err != nil {
		g.log.Error(err)
		return err
	}

	transactOpts, err := g.getTransactOpts(ctx, privKey)
	if err != nil {
		g.log.Error(err)
		return err
	}

	tx, err := instance.SetContractCloseOut(transactOpts, big.NewInt(int64(closeoutType)))
	if err != nil {
		g.log.Error(err)
		return err
	}

	_, err = bind.WaitMined(ctx, g.client, tx)
	if err != nil {
		g.log.Error(err)
		return err
	}

	return nil
}

func (s *HashrateEthereum) CreateCloneFactorySubscription(ctx context.Context, clonefactoryAddr common.Address) (*lib.Subscription, error) {
	return WatchContractEvents(ctx, s.client, clonefactoryAddr, CreateEventMapper(clonefactoryEventFactory, s.cfABI), SUBSCRIPTION_MAX_RECONNECTS, s.log)
}

func (s *HashrateEthereum) CreateImplementationSubscription(ctx context.Context, contractAddr common.Address) (*lib.Subscription, error) {
	return WatchContractEvents(ctx, s.client, contractAddr, CreateEventMapper(implementationEventFactory, s.implABI), SUBSCRIPTION_MAX_RECONNECTS, s.log)
}

func (g *HashrateEthereum) getTransactOpts(ctx context.Context, privKey string) (*bind.TransactOpts, error) {
	privateKey, err := crypto.HexToECDSA(privKey)
	if err != nil {
		return nil, err
	}

	chainId, err := g.client.ChainID(ctx)
	if err != nil {
		return nil, err
	}

	transactOpts, err := bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	if err != nil {
		return nil, err
	}

	// TODO: deal with likely gasPrice issue so our transaction processes before another pending nonce.
	if g.legacyTx {
		gasPrice, err := g.client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, err
		}
		transactOpts.GasPrice = gasPrice
	}

	fromAddr, err := lib.PrivKeyToAddr(privateKey)
	if err != nil {
		return nil, err
	}

	nonce, err := g.getNonce(ctx, fromAddr)
	if err != nil {
		return nil, err
	}

	transactOpts.Value = big.NewInt(0)
	transactOpts.Nonce = nonce
	transactOpts.Context = ctx

	return transactOpts, nil
}

func (s *HashrateEthereum) getNonce(ctx context.Context, from common.Address) (*big.Int, error) {
	// TODO: consider assuming that local cached nonce is correct and
	// only retrieve pending nonce from blockchain in case of unlikely error
	s.mutex.Lock()
	defer s.mutex.Unlock()

	nonce := &big.Int{}
	blockchainNonce, err := s.client.PendingNonceAt(ctx, from)
	if err != nil {
		return nonce, err
	}

	if s.nonce > blockchainNonce {
		nonce.SetUint64(s.nonce)
	} else {
		nonce.SetUint64(blockchainNonce)
	}

	s.nonce = nonce.Uint64() //+ 1

	return nonce, nil
}
