package contract

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/repositories/contracts"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources"
	hashrateContract "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/allocator"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
)

type ContractFactory struct {
	// config
	privateKey             string // private key of the user
	cycleDuration          time.Duration
	validationStartTimeout time.Duration
	shareTimeout           time.Duration
	hrErrorThreshold       float64
	hashrateErrorInterval  time.Duration

	// state
	address common.Address // derived from private key

	// deps
	store           *contracts.HashrateEthereum
	allocator       *allocator.Allocator
	globalHashrate  *hashrate.GlobalHashrate
	hashrateFactory func() *hashrate.Hashrate
	log             interfaces.ILogger
}

func NewContractFactory(
	allocator *allocator.Allocator,
	hashrateFactory func() *hashrate.Hashrate,
	globalHashrate *hashrate.GlobalHashrate,
	store *contracts.HashrateEthereum,
	log interfaces.ILogger,

	privateKey string,
	cycleDuration time.Duration,
	validationStartTimeout time.Duration,
	shareTimeout time.Duration,
	hrErrorThreshold float64,
	hashrateErrorInterval time.Duration,
) (*ContractFactory, error) {
	address, err := lib.PrivKeyStringToAddr(privateKey)
	if err != nil {
		return nil, err
	}

	return &ContractFactory{
		allocator:       allocator,
		hashrateFactory: hashrateFactory,
		globalHashrate:  globalHashrate,
		store:           store,
		log:             log,

		address: address,

		privateKey:             privateKey,
		cycleDuration:          cycleDuration,
		validationStartTimeout: validationStartTimeout,
		shareTimeout:           shareTimeout,
		hrErrorThreshold:       hrErrorThreshold,
		hashrateErrorInterval:  hashrateErrorInterval,
	}, nil
}

func (c *ContractFactory) CreateContract(contractData *hashrateContract.EncryptedTerms) (resources.Contract, error) {
	if contractData.Seller == c.address.String() {
		terms, err := contractData.Decrypt(c.privateKey)
		if err != nil {
			return nil, err
		}
		watcher := NewContractWatcherSeller(terms, c.cycleDuration, c.hashrateFactory, c.allocator, c.log.Named(lib.AddrShort(contractData.ContractID)))
		return NewControllerSeller(watcher, c.store, c.privateKey), nil
	}
	if contractData.Buyer == c.address.String() {
		watcher := NewContractWatcherBuyer(
			contractData,
			c.hashrateFactory,
			c.allocator,
			c.globalHashrate,
			c.log.Named(lib.AddrShort(contractData.ContractID)),

			c.cycleDuration,
			c.validationStartTimeout,
			c.shareTimeout,
			c.hrErrorThreshold,
			c.hashrateErrorInterval,
		)
		return NewControllerBuyer(watcher, c.store, c.privateKey), nil
	}
	return nil, fmt.Errorf("invalid terms %+v", contractData)
}

func (c *ContractFactory) GetType() resources.ResourceType {
	return ResourceTypeHashrate
}
