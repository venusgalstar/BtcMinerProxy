package proxy

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	sm "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/proxy/stratumv1_message"
)

func TestReadCancellation(t *testing.T) {
	delay := 50 * time.Millisecond
	timeout := 1 * time.Minute
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := CreateConnection(client, "", timeout, timeout, lib.NewTestLogger())

	go func() {
		// first and only write
		_, err := server.Write(append(sm.NewMiningAuthorize(0, "0", "0").Serialize(), lib.CharNewLine))
		if err != nil {
			t.Error(err)
		}
	}()

	// read first message ok
	_, err := conn.Read(ctx)
	require.NoError(t, err)

	go func() {
		time.Sleep(delay)
		cancel()
	}()

	// read second message should block, and then be cancelled
	t1 := time.Now()
	_, err = conn.Read(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.GreaterOrEqual(t, time.Since(t1), delay)
}

func TestWriteCancellation(t *testing.T) {
	delay := 50 * time.Millisecond
	timeout := 1 * time.Minute
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stratumClient := CreateConnection(client, "", timeout, timeout, lib.NewTestLogger())
	stratumServer := CreateConnection(server, "", timeout, timeout, lib.NewTestLogger())

	go func() {
		// first and only read
		_, err := stratumServer.Read(context.Background())
		if err != nil {
			t.Error(err)
		}
	}()

	// write first message ok
	err := stratumClient.Write(ctx, sm.NewMiningAuthorize(0, "0", "0"))
	require.NoError(t, err)

	go func() {
		time.Sleep(delay)
		cancel()
	}()

	// write second message should block, and then be cancelled
	t1 := time.Now()
	err = stratumClient.Write(ctx, sm.NewMiningAuthorize(1, "0", "0"))

	require.ErrorIs(t, err, context.Canceled)
	require.GreaterOrEqual(t, time.Since(t1), delay)
}

func TestConnTimeoutWrite(t *testing.T) {
	timeout := 50 * time.Millisecond
	timeoutLong := 1 * time.Second
	allowance := 50 * time.Millisecond
	server, client := net.Pipe()
	ctx := context.Background()

	defer server.Close()
	defer client.Close()

	clientConn := CreateConnection(client, "", timeoutLong, timeout, lib.NewTestLogger().Named("client"))
	serverConn := CreateConnection(server, "", timeoutLong, timeoutLong, lib.NewTestLogger().Named("server"))

	go func() {
		// try to read first message
		_, _ = serverConn.Read(ctx)
		// try to read second message, will fail due to timeout
		_, _ = serverConn.Read(ctx)
	}()

	// write first message ok
	err := clientConn.Write(ctx, sm.NewMiningAuthorize(0, "0", "0"))
	require.NoError(t, err)

	// sleep to reach timeout
	time.Sleep(timeout + allowance)

	// write second message, should fail due to timeout
	err = clientConn.Write(ctx, sm.NewMiningAuthorize(0, "0", "0"))
	require.ErrorIs(t, err, io.ErrClosedPipe) // in real connections it is going to be io.EOF

	// try to read message, should fail as well due to timeout
	_, err = clientConn.Read(ctx)
	require.ErrorIs(t, err, io.ErrClosedPipe) // in real connections it is going to be io.EOF
}

func TestConnTimeoutRead(t *testing.T) {
	timeout := 50 * time.Millisecond
	timeoutLong := 1 * time.Second
	allowance := 50 * time.Millisecond
	server, client := net.Pipe()
	ctx := context.Background()

	defer server.Close()
	defer client.Close()

	clientConn := CreateConnection(client, "", timeout, timeoutLong, lib.NewTestLogger().Named("client"))
	serverConn := CreateConnection(server, "", timeoutLong, timeoutLong, lib.NewTestLogger().Named("server"))

	go func() {
		_ = serverConn.Write(ctx, sm.NewMiningAuthorize(0, "0", "0"))
		_ = serverConn.Write(ctx, sm.NewMiningAuthorize(0, "0", "0"))
	}()

	// read first message ok
	_, err := clientConn.Read(ctx)
	require.NoError(t, err)

	// sleep to reach timeout
	time.Sleep(timeout + allowance)

	// read second message, should fail due to timeout
	_, err = clientConn.Read(ctx)
	require.ErrorIs(t, err, io.ErrClosedPipe) // in real connections it is going to be io.EOF

	// try to read message, should fail as well due to timeout
	err = clientConn.Write(ctx, sm.NewMiningAuthorize(0, "0", "0"))
	require.ErrorIs(t, err, io.ErrClosedPipe) // in real connections it is going to be io.EOF
}
