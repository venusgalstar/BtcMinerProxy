package hashrate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
)

func TestEncryptDecrypt(t *testing.T) {
	privateKey := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	msg := "zuck"

	privateKey2, err := crypto.HexToECDSA(privateKey)
	require.NoError(t, err)

	publicKey, ok := privateKey2.Public().(*ecdsa.PublicKey)
	require.True(t, ok)

	publicKeyBytes := elliptic.Marshal(publicKey.Curve, publicKey.X, publicKey.Y)
	hx := hex.EncodeToString(publicKeyBytes)

	encoded, err := lib.EncryptString(msg, hx)
	require.NoError(t, err)

	decoded, err := lib.DecryptString(encoded, privateKey)
	require.NoError(t, err)

	require.Equal(t, msg, decoded)
}
