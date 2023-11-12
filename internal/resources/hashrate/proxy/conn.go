package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	gi "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/proxy/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/proxy/stratumv1_message"
)

const (
	DIAL_TIMEOUT  = 10 * time.Second
	WRITE_TIMEOUT = 10 * time.Second

	READ_CLOSE_TIMEOUT  = 10 * time.Minute
	WRITE_CLOSE_TIMEOUT = 10 * time.Minute
)

type StratumConnection struct {
	// config
	id string

	// connTimeout      time.Duration
	// connection will automatically close if no read (write) operation is performed for this duration
	// the read/write operation will return
	connReadTimeout  time.Duration
	connWriteTimeout time.Duration

	address string

	// state
	connectedAt   time.Time
	reader        *bufio.Reader
	timeoutOnce   sync.Once
	readHappened  chan struct{}
	writeHappened chan struct{}
	closedCh      chan struct{}
	closeOnce     sync.Once

	idleReadAtNano  atomic.Int64 // unixNano time when connection is going to close due to idle read (no read operation for READ_CLOSE_TIMEOUT)
	idleWriteAtNano atomic.Int64 // unixNano time when connection is going to close due to idle write (no write operation for WRITE_CLOSE_TIMEOUT)

	// deps
	conn net.Conn
	log  gi.ILogger
}

// CreateConnection creates a new StratumConnection and starts background timer for its closure
func CreateConnection(conn net.Conn, address string, readTimeout, writeTimeout time.Duration, log gi.ILogger) *StratumConnection {
	c := &StratumConnection{
		id:               address,
		conn:             conn,
		address:          address,
		connectedAt:      time.Now(),
		connReadTimeout:  readTimeout,
		connWriteTimeout: writeTimeout,
		reader:           bufio.NewReader(conn),
		readHappened:     make(chan struct{}, 1),
		writeHappened:    make(chan struct{}, 1),
		closedCh:         make(chan struct{}),
		log:              log,
	}
	err := conn.SetDeadline(time.Now().Add(1 * time.Hour))
	if err != nil {
		panic(err)
	}
	c.runTimeoutTimers()
	return c
}

// Connect connects to destination with default close timeouts
func Connect(address *url.URL, log gi.ILogger) (*StratumConnection, error) {
	conn, err := net.DialTimeout("tcp", address.Host, DIAL_TIMEOUT)
	if err != nil {
		return nil, err
	}

	return CreateConnection(conn, address.String(), READ_CLOSE_TIMEOUT, WRITE_CLOSE_TIMEOUT, log), nil
}

func (c *StratumConnection) Read(ctx context.Context) (interfaces.MiningMessageGeneric, error) {
	doneCh := make(chan struct{})
	defer close(doneCh)

	// cancellation via context is implemented using SetReadDeadline,
	// which unblocks read operation causing it to return os.ErrDeadlineExceeded
	// TODO: consider implementing it in separate goroutine instead of a goroutine per read
	go func() {
		select {
		case <-ctx.Done():
			c.log.Debugf("connection %s read cancelled", c.id)
			err := c.conn.SetReadDeadline(time.Now())
			if err != nil {
				// may return ErrNetClosing if fd is already closed
				c.log.Warnf("err during setting read deadline: %s", err)
				return
			}
		case <-doneCh:
			return
		}
	}()

	err := c.conn.SetReadDeadline(time.Time{})

	if err != nil {
		return nil, err
	}

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		line, isPrefix, err := c.reader.ReadLine()

		if isPrefix {
			return nil, fmt.Errorf("line is too long for the buffer used: %s", string(line))
		}

		if err != nil {
			// if read was cancelled via context return context error, not deadline exceeded
			if ctx.Err() != nil && errors.Is(err, os.ErrDeadlineExceeded) {
				return nil, ctx.Err()
			}
			return nil, err
		}

		c.readHappened <- struct{}{}
		c.log.Debugf("<= %s", string(line))

		m, err := stratumv1_message.ParseStratumMessage(line)

		if errors.Is(err, stratumv1_message.ErrStratumV1Unknown) {
			c.log.Warnf("unknown stratum message, ignoring: %s", string(line))
			continue
		}

		if err != nil {
			err2 := fmt.Errorf("invalid stratum message: %s", string(line))
			return nil, lib.WrapError(err2, err)
		}

		return m, nil
	}
}

// Write writes message to the connection. Safe for concurrent use, cause underlying TCPConn is thread-safe
func (c *StratumConnection) Write(ctx context.Context, msg interfaces.MiningMessageGeneric) error {
	if msg == nil {
		return fmt.Errorf("nil message write attempt")
	}

	b := append(msg.Serialize(), lib.CharNewLine)

	doneCh := make(chan struct{})
	defer close(doneCh)

	err := c.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, WRITE_TIMEOUT)

	// cancellation via context is implemented using SetReadDeadline,
	// which unblocks read operation causing it to return os.ErrDeadlineExceeded
	// TODO: consider implementing it in separate goroutine instead of a goroutine per read
	go func() {
		select {
		case <-ctx.Done():
			err := c.conn.SetWriteDeadline(time.Now())
			if err != nil {
				// may return ErrNetClosing if fd is already closed
				c.log.Warnf("err during setting write deadline: %s", err)
				return
			}
		case <-doneCh:
			cancel()
			return
		}
	}()

	_, err = c.conn.Write(b)

	if err != nil {
		// if read was cancelled via context return context error, not deadline exceeded
		if ctx.Err() != nil && errors.Is(err, os.ErrDeadlineExceeded) {
			return ctx.Err()
		}
		return err
	}

	c.writeHappened <- struct{}{}

	c.log.Debugf("=> %s", string(msg.Serialize()))

	return nil
}

func (c *StratumConnection) GetID() string {
	return c.id
}

func (c *StratumConnection) Close() error {
	err := c.conn.Close()
	if err == nil {
		c.log.Infof("connection closed %s", c.id)
	} else {
		c.log.Warnf("connection already closed %s", c.id)
	}

	c.closeOnce.Do(func() {
		close(c.closedCh)
	})

	return err
}

func (c *StratumConnection) GetConnectedAt() time.Time {
	return c.connectedAt
}

func (c *StratumConnection) GetIdleCloseAt() time.Time {
	idleReadAt := time.Unix(0, c.idleReadAtNano.Load())
	idleWriteAt := time.Unix(0, c.idleWriteAtNano.Load())

	if idleReadAt.After(idleWriteAt) {
		return idleReadAt
	}
	return idleWriteAt
}

func (c *StratumConnection) ResetIdleCloseTimers() {
	c.readHappened <- struct{}{}
	c.writeHappened <- struct{}{}
}

// runTimeoutTimers runs timers to close inactive connections. If no read or write operation
// is performed for the specified duration defined separately for read and write, connection will close
func (c *StratumConnection) runTimeoutTimers() {
	c.timeoutOnce.Do(func() {
		go func() {
			readTimer, writeTimer := time.NewTimer(c.connReadTimeout), time.NewTimer(c.connWriteTimeout)

			for {
				select {
				case <-readTimer.C:
					c.log.Info("connection read timeout")
					if !writeTimer.Stop() {
						<-writeTimer.C
					}
					c.Close()
					return
				case <-writeTimer.C:
					c.log.Info("connection write timeout")
					if !readTimer.Stop() {
						<-readTimer.C
					}
					c.Close()
					return
				case <-c.readHappened:
					if !readTimer.Stop() {
						<-readTimer.C
					}
					readTimer.Reset(c.connReadTimeout)
					c.idleReadAtNano.Store(time.Now().Add(c.connReadTimeout).UnixNano())
				case <-c.writeHappened:
					if !writeTimer.Stop() {
						<-writeTimer.C
					}
					writeTimer.Reset(c.connWriteTimeout)
					c.idleWriteAtNano.Store(time.Now().Add(c.connWriteTimeout).UnixNano())
				case <-c.closedCh:
					if !readTimer.Stop() {
						<-readTimer.C
					}
					if !writeTimer.Stop() {
						<-writeTimer.C
					}
					return
				}
			}
		}()
	})
}
