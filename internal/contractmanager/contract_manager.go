package contractmanager

import (
	"context"
	"sync"

	"github.com/Lumerin-protocol/contracts-go/clonefactory"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/repositories/contracts"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
)

type ContractManager struct {
	cfAddr    common.Address
	ownerAddr common.Address

	contracts   *lib.Collection[resources.Contract]
	contractsWG sync.WaitGroup

	contractFactory ContractFactory
	store           *contracts.HashrateEthereum
	log             interfaces.ILogger
}

type ContractFactory func(terms *hashrate.EncryptedTerms) (resources.Contract, error)

func NewContractManager(clonefactoryAddr, ownerAddr common.Address, contractFactory ContractFactory, store *contracts.HashrateEthereum, log interfaces.ILogger) *ContractManager {
	return &ContractManager{
		cfAddr:          clonefactoryAddr,
		ownerAddr:       ownerAddr,
		contracts:       lib.NewCollection[resources.Contract](),
		contractFactory: contractFactory,
		store:           store,
		contractsWG:     sync.WaitGroup{},
		log:             log,
	}
}

func (cm *ContractManager) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer cm.contractsWG.Wait()

	contractIDs, err := cm.store.GetContractsIDs(ctx)
	if err != nil {
		return err
	}

	for _, id := range contractIDs {
		terms, err := cm.store.GetContract(ctx, id)
		if err != nil {
			return err
		}
		if cm.isOurContract(terms) {
			cm.AddContract(ctx, terms)
		}
	}

	sub, err := cm.store.CreateCloneFactorySubscription(ctx, cm.cfAddr)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()
	cm.log.Infof("subscribed to clonefactory events, address %s", cm.cfAddr.Hex())

	for {
		select {
		case <-ctx.Done():
			//TODO: wait until all child contracts are stopped
			return nil
		case event := <-sub.Events():
			err := cm.cloneFactoryController(ctx, event)
			if err != nil {
				return err
			}
		case err := <-sub.Err():
			return err
		}
	}
}

func (cm *ContractManager) cloneFactoryController(ctx context.Context, event interface{}) error {
	switch e := event.(type) {
	case *clonefactory.ClonefactoryContractCreated:
		return cm.handleContractCreated(ctx, e)
	case *clonefactory.ClonefactoryClonefactoryContractPurchased:
		return cm.handleContractPurchased(ctx, e)
	case *clonefactory.ClonefactoryContractDeleteUpdated:
		return cm.handleContractDeleteUpdated(ctx, e)
	}
	return nil
}

func (cm *ContractManager) handleContractCreated(ctx context.Context, event *clonefactory.ClonefactoryContractCreated) error {
	terms, err := cm.store.GetContract(ctx, event.Address.Hex())
	if err != nil {
		return err
	}
	if cm.isOurContract(terms) {
		cm.AddContract(ctx, terms)
	}
	return nil
}

func (cm *ContractManager) handleContractPurchased(ctx context.Context, event *clonefactory.ClonefactoryClonefactoryContractPurchased) error {
	terms, err := cm.store.GetContract(ctx, event.Address.Hex())
	if err != nil {
		return err
	}
	if terms.GetBuyer() == cm.ownerAddr.String() {
		cm.AddContract(ctx, terms)
	}
	return nil
}

func (cm *ContractManager) handleContractDeleteUpdated(ctx context.Context, event *clonefactory.ClonefactoryContractDeleteUpdated) error {
	// TODO: handle contract delete / undelete
	// as of now all contracts are tracked regardless of delete status
	return nil
}

func (cm *ContractManager) AddContract(ctx context.Context, data *hashrate.EncryptedTerms) {
	_, ok := cm.contracts.Load(data.GetID())
	if ok {
		cm.log.Error("contract already exists in store")
		return
	}

	cntr, err := cm.contractFactory(data)
	if err != nil {
		cm.log.Errorf("contract factory error %s", err)
		return
	}

	cm.contracts.Store(cntr)

	cm.contractsWG.Add(1)
	go func() {
		defer cm.contractsWG.Done()

		err := cntr.Run(ctx)
		if err != nil {
			cm.log.Warn(err)
		}

		cm.contracts.Delete(cntr.GetID())
	}()
}

func (cm *ContractManager) GetContracts() *lib.Collection[resources.Contract] {
	return cm.contracts
}

func (cm *ContractManager) isOurContract(terms TermsCommon) bool {
	return terms.GetSeller() == cm.ownerAddr.String() || terms.GetBuyer() == cm.ownerAddr.String()
}
