package contracts

import (
	"context"

	"github.com/Lumerin-protocol/contracts-go/clonefactory"
	"github.com/Lumerin-protocol/contracts-go/implementation"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
)

type EventMapper func(types.Log) (interface{}, error)

func implementationEventFactory(name string) interface{} {
	switch name {
	case "contractPurchased":
		return new(implementation.ImplementationContractPurchased)
	case "contractClosed":
		return new(implementation.ImplementationContractClosed)
	case "cipherTextUpdated":
		return new(implementation.ImplementationCipherTextUpdated)
	default:
		return nil
	}
}

func clonefactoryEventFactory(name string) interface{} {
	switch name {
	case "contractCreated":
		return new(clonefactory.ClonefactoryContractCreated)
	case "clonefactoryContractPurchased":
		return new(clonefactory.ClonefactoryClonefactoryContractPurchased)
	case "purchaseInfoUpdated":
		return new(clonefactory.ClonefactoryPurchaseInfoUpdated)
	case "contractDeleteUpdated":
		return new(clonefactory.ClonefactoryContractDeleteUpdated)
	default:
		return nil
	}
}

// WatchContractEvents watches for all events from the contract and converts them to the concrete type, using mapper
func WatchContractEvents(ctx context.Context, client EthereumClient, contractAddr common.Address, mapper EventMapper, maxReconnects int, log interfaces.ILogger) (*lib.Subscription, error) {
	sink := make(chan interface{})

	return lib.NewSubscription(func(quit <-chan struct{}) error {
		defer close(sink)

		query := ethereum.FilterQuery{
			Addresses: []common.Address{contractAddr},
		}
		in := make(chan types.Log)
		defer close(in)

		var lastErr error

		for attempts := 0; attempts < maxReconnects; attempts++ {
			sub, err := client.SubscribeFilterLogs(ctx, query, in)
			if err != nil {
				lastErr = err
				continue
			}
			if attempts > 0 {
				log.Warnf("subscription reconnected due to error: %s", lastErr)
			}
			attempts = 0

			defer sub.Unsubscribe()

		EVENTS_LOOP:
			for {
				select {
				case log := <-in:
					event, err := mapper(log)
					if err != nil {
						// mapper error, retry won't help
						return err
					}

					select {
					case sink <- event:
					case err := <-sub.Err():
						lastErr = err
						break EVENTS_LOOP
					case <-quit:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				case err := <-sub.Err():
					lastErr = err
					break EVENTS_LOOP
				case <-quit:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}

		return lastErr
	}, sink), nil
}
