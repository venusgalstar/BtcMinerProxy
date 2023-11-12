package lib

import (
	"fmt"
	"math/big"
	"math/rand"

	"github.com/ethereum/go-ethereum/common"
)

func GetRandomAddr() common.Address {
	return common.BigToAddress(big.NewInt(rand.Int63()))
}

func AddrShort(addr string) string {
	ox, prefix, suffix := 2, 3, 3
	l := len(addr)
	if l >= (prefix + suffix + 2) {
		return fmt.Sprintf("0x%s..%s", addr[ox:ox+prefix], addr[l-suffix:l])
	}
	return addr
}
