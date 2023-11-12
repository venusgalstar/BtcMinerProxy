package contract

import (
	"context"
	"errors"

	"github.com/Lumerin-protocol/contracts-go/implementation"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/repositories/contracts"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources"
)

type ControllerBuyer struct {
	*ContractWatcherBuyer
	store   *contracts.HashrateEthereum
	tsk     *lib.Task
	privKey string
}

func NewControllerBuyer(contract *ContractWatcherBuyer, store *contracts.HashrateEthereum, privKey string) *ControllerBuyer {
	return &ControllerBuyer{
		ContractWatcherBuyer: contract,
		store:                store,
		privKey:              privKey,
	}
}

func (c *ControllerBuyer) Run(ctx context.Context) error {
	sub, err := c.store.CreateImplementationSubscription(ctx, common.HexToAddress(c.GetID()))
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()
	c.log.Infof("started watching contract as buyer, address %s", c.GetID())

	c.ContractWatcherBuyer.StartFulfilling(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-sub.Events():
			err := c.controller(ctx, event)
			if err != nil {
				return err
			}
		case err := <-sub.Err():
			return err
		case <-c.ContractWatcherBuyer.Done():
			err := c.ContractWatcherBuyer.Err()
			if err != nil {

				// contract closed, no need to close it again
				if errors.Is(err, ErrContractClosed) {
					c.log.Warnf("buyer contract ended due to closeout")
					return nil
				}

				// underdelivery, buyer closes the contract
				c.log.Warnf("buyer contract ended with error: %s", err)
				err = c.store.CloseContract(ctx, c.GetID(), contracts.CloseoutTypeCancel, c.privKey)
				if err != nil {
					c.log.Errorf("error closing contract: %s", err)
				}
				c.log.Warnf("buyer contract closed, with type cancel")
				return nil
			}
			// delivery ok, seller will close the contract
			c.log.Infof("buyer contract ended without an error")
			return nil
		}
	}
}

func (c *ControllerBuyer) controller(ctx context.Context, event interface{}) error {
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

func (c *ControllerBuyer) handleContractPurchased(ctx context.Context, event *implementation.ImplementationContractPurchased) error {
	return nil
}

func (c *ControllerBuyer) handleContractClosed(ctx context.Context, event *implementation.ImplementationContractClosed) error {
	if c.GetState() == resources.ContractStateRunning {
		c.StopFulfilling()
	}

	return nil
}

func (c *ControllerBuyer) handleCipherTextUpdated(ctx context.Context, event *implementation.ImplementationCipherTextUpdated) error {
	// ignoring, if destination cipher is changed then there is going to be a different destination
	return nil
}

func (c *ControllerBuyer) handlePurchaseInfoUpdated(ctx context.Context, event *implementation.ImplementationPurchaseInfoUpdated) error {
	// this event is emitted only when contract is closed, so we can ignore it
	// and pull updated terms on the next purchase
	return nil
}
