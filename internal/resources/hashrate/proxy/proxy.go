package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	gi "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
)

const (
	CONNECTION_TIMEOUT = 10 * time.Minute
	RESPONSE_TIMEOUT   = 30 * time.Second
)

var (
	ErrConnectDest       = errors.New("failure during connecting to destination")
	ErrConnectSource     = errors.New("failure during source connection")
	ErrHandshakeDest     = errors.New("failure during handshake with destination")
	ErrHandshakeSource   = errors.New("failure during handshake with source")
	ErrProxy             = errors.New("proxy error")
	ErrNotAuthorizedPool = errors.New("not authorized in the pool")
	ErrChangeDest        = errors.New("destination change error")
)

type Proxy struct {
	// config
	ID                     string
	destURL                *url.URL // destination URL, TODO: remove, use dest.ID() instead
	notPropagateWorkerName bool

	// destWorkerName string
	// submitErrLimit int
	// onFault        func(context.Context) // called when proxy becomes faulty (e.g. when submit error limit is reached

	// state
	destToSourceStartSignal chan struct{}      // signal to start reading from destination
	hashrate                *hashrate.Hashrate // hashrate of the source validated by the proxy
	pipe                    *Pipe
	cancelRun               context.CancelFunc         // cancels Run() task
	setDestLock             sync.Mutex                 // mutex to protect SetDest() from concurrent calls
	unansweredMsg           sync.WaitGroup             // number of unanswered messages from the source
	onSubmit                HashrateCounterFunc        // callback to update contract hashrate
	onSubmitMutex           sync.RWMutex               // mutex to protect onSubmit
	destMap                 *lib.Collection[*ConnDest] // map of all available destinations (pools) currently connected to the single source (miner)

	// deps
	source         *ConnSource           // initiator of the communication, miner
	dest           *ConnDest             // receiver of the communication, pool
	globalHashrate GlobalHashrateCounter // callback to update global hashrate per worker
	destFactory    DestConnFactory       // factory to create new destination connections
	log            gi.ILogger
}

// TODO: pass connection factory for destURL
func NewProxy(ID string, source *ConnSource, destFactory DestConnFactory, hashrateFactory HashrateFactory, globalHashrate GlobalHashrateCounter, destURL *url.URL, notPropagateWorkerName bool, log gi.ILogger) *Proxy {
	proxy := &Proxy{
		ID:                     ID,
		destURL:                destURL,
		notPropagateWorkerName: notPropagateWorkerName,

		source:      source,
		destMap:     lib.NewCollection[*ConnDest](),
		destFactory: destFactory,
		log:         log,

		hashrate:       hashrateFactory(),
		globalHashrate: globalHashrate,
		onSubmit:       nil,
	}

	return proxy
}

var (
	minerSubscribeReceived = false
	//TODO: enforce message order validation
)

// runs proxy until handshake is done
func (p *Proxy) Connect(ctx context.Context) error {
	err := NewHandlerFirstConnect(p, p.log).Connect(ctx)
	if err != nil {
		p.closeConnections()
		return err
	}
	return nil
}

func (p *Proxy) Run(ctx context.Context) error {
	defer p.closeConnections()

	handler := NewHandlerMining(p, p.log)

	p.pipe = NewPipe(p.source, p.dest, handler.sourceInterceptor, handler.destInterceptor, p.log)
	p.pipe.StartSourceToDest(ctx)
	p.pipe.StartDestToSource(ctx)

	ctx, cancel := context.WithCancel(ctx)
	p.cancelRun = cancel

	err := p.pipe.Run(ctx)
	p.unansweredMsg.Wait()

	if err != nil {
		// destination error
		if errors.Is(err, ErrDest) {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				p.log.Warnf("destination closed the connection, dest %s", p.dest.GetID())
			} else {
				p.log.Errorf("destination error, source %s dest %s: %s", p.source.GetID(), p.dest.GetID(), err)
			}

			return err
			// TODO: reconnect to the same dest
			// return p.SetDest(ctx, p.destURL, p.onSubmit)
		}

		// source error
		if errors.Is(err, ErrSource) {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				p.log.Warnf("source closed the connection, source %s", p.source.GetID())
			} else {
				p.log.Errorf("source connection error, source %s: %s", p.source.GetID(), err)
			}
			return err
		}

		if errors.Is(err, context.Canceled) {
			p.log.Warnf("proxy stopped %s", p.ID)
			return err
		}

		p.log.Errorf("error running pipe: %s", err)

		// other errors
		return err
	}
	return nil
}

func (p *Proxy) SetDest(ctx context.Context, newDestURL *url.URL, onSubmit func(diff float64)) error {
	p.setDestLock.Lock()
	defer p.setDestLock.Unlock()

	if p.destURL.String() == newDestURL.String() {
		p.log.Debugf("changing destination skipped, because it is the same as current")
		return nil
	}

	p.log.Debugf("changing destination to %s", newDestURL.String())
	destChanger := NewHandlerChangeDest(p, p.destFactory, p.log)

	var newDest *ConnDest
	cachedDest, ok := p.destMap.Load(newDestURL.String())
	if ok {
		p.log.Debugf("reusing dest connection %s from cache", newDestURL.String())
		// limit waiting time, disconnect if not answered in time
		p.unansweredMsg.Wait()
		err := cachedDest.AutoReadStop()
		if err != nil {
			p.log.Errorf("error stopping autoread for cached dest %s: %s", newDestURL.String(), err)
			return err
		}
		cachedDest.ResetIdleCloseTimers()
		newDest = cachedDest
	} else {
		p.log.Debugf("connecting to new dest %s", newDestURL.String())
		dest, err := destChanger.connectNewDest(ctx, newDestURL)
		if err != nil {
			return err
		}

		p.unansweredMsg.Wait()
		newDest = dest
	}

	// stop source and old dest
	<-p.pipe.StopDestToSource()
	<-p.pipe.StopSourceToDest()
	p.log.Debugf("stopped source and old dest")

	// TODO: wait to stop?

	// set old dest to autoread mode
	destUrl := p.destURL.String()
	dest := p.dest
	err := dest.AutoReadStart(ctx, func(err error) {
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				p.log.Warnf("autoread exited with error %s", err)
				err := dest.conn.Close()
				if err != nil {
					p.log.Warnf("error closing dest %s: %s", destUrl, err)
				}
			}
		}

		p.destMap.Delete(destUrl)
	})
	if err != nil {
		return err
	}
	p.log.Debugf("set old dest to autoread")

	err = destChanger.resendRelevantNotifications(ctx, newDest)
	if err != nil {
		return err
	}

	p.dest = newDest
	p.destURL = newDestURL
	p.destMap.Store(newDest)

	p.onSubmitMutex.Lock()
	p.onSubmit = onSubmit
	p.onSubmitMutex.Unlock()

	p.pipe.SetDest(newDest)

	p.pipe.StartSourceToDest(ctx)
	p.pipe.StartDestToSource(ctx)

	p.log.Infof("destination changed to %s", newDestURL.String())
	return nil
}

func (p *Proxy) closeConnections() {
	if p.dest != nil {
		p.dest.conn.Close()
	}
	p.destMap.Delete(p.destURL.String())

	p.destMap.Range(func(dest *ConnDest) bool {
		dest.conn.Close()
		p.destMap.Delete(dest.GetID())
		return true
	})
}

func (p *Proxy) GetDestByJobID(jobID string) *ConnDest {
	var dest *ConnDest

	p.destMap.Range(func(d *ConnDest) bool {
		if d.HasJob(jobID) {
			dest = d
			return false
		}
		return true
	})

	return dest
}

// Getters
func (p *Proxy) GetID() string {
	return p.ID
}

func (p *Proxy) GetMinerConnectedAt() time.Time {
	return p.source.GetConnectedAt()
}

func (p *Proxy) GetDest() *url.URL {
	return p.destURL
}

func (p *Proxy) GetDestWorkerName() string {
	return p.destURL.User.Username()
}

func (p *Proxy) GetDifficulty() float64 {
	if p.dest == nil {
		return 0.0
	}
	return p.dest.GetDiff()
}

func (p *Proxy) GetHashrate() Hashrate {
	return p.hashrate
}

func (p *Proxy) GetConnectedAt() time.Time {
	return p.source.GetConnectedAt()
}

func (p *Proxy) GetSourceWorkerName() string {
	return p.source.GetUserName()
}

func (p *Proxy) GetStats() map[string]int {
	return p.source.GetStats().GetStatsMap()
}

func (p *Proxy) GetDestConns() *map[string]string {
	var destConns = make(map[string]string)
	p.destMap.Range(func(dest *ConnDest) bool {
		destConns[dest.GetID()] = dest.GetIdleCloseAt().Format(time.RFC3339)
		return true
	})
	return &destConns
}
