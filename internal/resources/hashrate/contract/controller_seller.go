package contract

import (
	"context"
	"time"

	"github.com/Lumerin-protocol/contracts-go/implementation"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/repositories/contracts"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources"

	hashrateContract "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
)

type ControllerSeller struct {
	*ContractWatcherSeller

	store   *contracts.HashrateEthereum
	privKey string
}

func NewControllerSeller(contract *ContractWatcherSeller, store *contracts.HashrateEthereum, privKey string) *ControllerSeller {
	return &ControllerSeller{
		ContractWatcherSeller: contract,
		store:                 store,
		privKey:               privKey,
	}
}

func (c *ControllerSeller) Run(ctx context.Context) error {
	sub, err := c.store.CreateImplementationSubscription(ctx, common.HexToAddress(c.GetID()))
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	c.log.Infof("started watching contract as seller, address %s", c.GetID())

	if c.ShouldBeRunning() {
		c.StartFulfilling(ctx)
	}

	for {
		select {
		case event := <-sub.Events():
			err := c.controller(ctx, event)
			if err != nil {
				c.StopFulfilling()
				return err
			}
		case err := <-sub.Err():
			c.StopFulfilling()
			return err
		case <-c.ContractWatcherSeller.Done():
			err := c.ContractWatcherSeller.Err()
			c.ContractWatcherSeller.Reset()
			if err != nil {
				// fulfillment error, buyer will close on underdelivery
				c.log.Warnf("seller contract ended with error: %s", err)
				continue
			}

			// no error, seller closes the contract after expiration
			c.log.Infof("seller contract ended without error")

			waitBeforeClose := 10 * time.Second
			c.log.Infof("sleeping %s", waitBeforeClose)
			time.Sleep(waitBeforeClose)

			c.log.Infof("closing contract id %s, startsAt %s, duration %s, elapsed %s", c.GetID(), c.GetStartedAt(), c.GetDuration(), c.GetElapsed())
			err = c.store.CloseContract(ctx, c.GetID(), contracts.CloseoutTypeWithoutClaim, c.privKey)

			if err != nil {
				c.log.Errorf("error closing contract: %s", err)
			} else {
				c.log.Warnf("seller contract closed")
			}
		}
	}
}

func (c *ControllerSeller) controller(ctx context.Context, event interface{}) error {
	switch e := event.(type) {
	case *implementation.ImplementationContractPurchased:
		return c.handleContractPurchased(ctx, e)
	case *implementation.ImplementationContractClosed:
		return c.handleContractClosed(ctx, e)
	case *implementation.ImplementationCipherTextUpdated:
		return c.handleCipherTextUpdated(ctx, e)
	case *implementation.ImplementationPurchaseInfoUpdated:
		return c.handlePurchaseInfoUpdated(ctx, e)
	}
	return nil
}

func (c *ControllerSeller) handleContractPurchased(ctx context.Context, event *implementation.ImplementationContractPurchased) error {
	c.log.Debugf("got purchased event for contract %s", c.GetID())
	if c.GetState() == resources.ContractStateRunning {
		return nil
	}

	err := c.LoadTermsFromBlockchain(ctx)

	if err != nil {
		return err
	}

	c.StartFulfilling(ctx)

	return nil
}

func (c *ControllerSeller) LoadTermsFromBlockchain(ctx context.Context) error {

	terms, err := c.GetTermsFromBlockchain(ctx)

	if err != nil {
		return err
	}

	c.SetData(terms)

	return nil
}

func (c *ControllerSeller) GetTermsFromBlockchain(ctx context.Context) (*hashrateContract.Terms, error) {
	encryptedTerms, err := c.store.GetContract(ctx, c.GetID())

	if err != nil {
		return nil, err
	}

	terms, err := encryptedTerms.Decrypt(c.privKey)

	if err != nil {
		return nil, err
	}

	return terms, nil
}

func (c *ControllerSeller) handleContractClosed(ctx context.Context, event *implementation.ImplementationContractClosed) error {
	c.log.Warnf("got closed event for contract")
	if c.GetState() == resources.ContractStateRunning {
		c.StopFulfilling()
	}

	err := c.LoadTermsFromBlockchain(ctx)

	if err != nil {
		return err
	}

	return nil
}

func (c *ControllerSeller) handleCipherTextUpdated(ctx context.Context, event *implementation.ImplementationCipherTextUpdated) error {
	currentDest := c.GetDest()

	terms, err := c.GetTermsFromBlockchain(ctx)

	if err != nil {
		return err
	}

	//TODO: drop protocol before comparison
	newDest := terms.Dest.String()

	if currentDest == newDest {
		return nil
	}

	c.ContractWatcherSeller.StopFulfilling()
	c.SetData(terms)
	c.ContractWatcherSeller.StartFulfilling(ctx)
	return nil
}

func (c *ControllerSeller) handlePurchaseInfoUpdated(ctx context.Context, event *implementation.ImplementationPurchaseInfoUpdated) error {
	err := c.LoadTermsFromBlockchain(ctx)

	if err != nil {
		return err
	}

	return nil
}
