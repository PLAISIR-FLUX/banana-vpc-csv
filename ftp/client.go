package ftp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type Client struct {
	config          Config
	hosts           []string
	freeConnCh      chan *persistentConn
	numConnsPerHost map[string]int
	allCons         map[int]*persistentConn
	connIdx         int
	rawConnIdx      int
	mu              sync.Mutex
	t0              time.Time
	closed          bool
}

type TLSMode int

const (
	TLSExplicit TLSMode = 0
	TLSImplicit TLSMode = 1
)

type stubResponse struct {
	code int
	msg  string
}

type Config struct {
	User               string
	Password           string
	ConnectionsPerHost int
	Timeout            time.Duration
	TLSConfig          *tls.Config
	TLSMode            TLSMode
	IPv6Lookup         bool
	Logger             io.Writer
	ServerLocation     *time.Location
	ActiveTransfers    bool
	ActiveListenAddr   string
	DisableEPSV        bool
	stubResponses      map[string]stubResponse
}

type Error interface {
	error
	Temporary() bool
	Code() int
	Message() string
}

type ftpError struct {
	err       error
	code      int
	msg       string
	timeout   bool
	temporary bool
}

func (e ftpError) Error() string {
	if e.err != nil {
		return e.err.Error()
	} else {
		return fmt.Sprintf("unexpected response: %d-%s", e.code, e.msg)
	}
}

func (e ftpError) Temporary() bool {
	return e.temporary || transientNegativeCompletionReply(e.code)
}

func (e ftpError) Timeout() bool {
	return e.timeout
}

func (e ftpError) Code() int {
	if fe, _ := e.err.(Error); fe != nil {
		return fe.Code()
	}
	return e.code
}

func (e ftpError) Message() string {
	if fe, _ := e.err.(Error); fe != nil {
		return fe.Message()
	}
	return e.msg
}

func newClient(config Config, hosts []string) *Client {
	if config.ConnectionsPerHost <= 0 {
		config.ConnectionsPerHost = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.User == "" {
		config.User = "anonymous"
	}
	if config.Password == "" {
		config.Password = "anonymous"
	}
	if config.ServerLocation == nil {
		config.ServerLocation = time.UTC
	}
	if config.ActiveListenAddr == "" {
		config.ActiveListenAddr = ":0"
	}
	return &Client{
		config:          config,
		freeConnCh:      make(chan *persistentConn, len(hosts)*config.ConnectionsPerHost),
		t0:              time.Now(),
		hosts:           hosts,
		allCons:         make(map[int]*persistentConn),
		numConnsPerHost: make(map[string]int),
	}
}

func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ftpError{err: errors.New("already closed")}
	}
	c.closed = true
	var conns []*persistentConn
	for _, conn := range c.allCons {
		conns = append(conns, conn)
	}
	c.mu.Unlock()
	for _, pconn := range conns {
		c.removeConn(pconn)
	}
	return nil
}

func (c *Client) debug(f string, args ...interface{}) {
	if c.config.Logger == nil {
		return
	}
	fmt.Fprintf(c.config.Logger, "goftp: %.3f %s\n",
		time.Now().Sub(c.t0).Seconds(),
		fmt.Sprintf(f, args...),
	)
}

func (c *Client) numOpenConns() int {
	var numOpen int
	for _, num := range c.numConnsPerHost {
		numOpen += int(num)
	}
	return numOpen
}

func (c *Client) getIdleConn() (*persistentConn, error) {
Loop:
	for {
		select {
		case pconn := <-c.freeConnCh:
			if pconn.broken {
				c.debug("#%d was ready (broken)", pconn.idx)
				c.mu.Lock()
				c.numConnsPerHost[pconn.host]--
				c.mu.Unlock()
				c.removeConn(pconn)
			} else {
				c.debug("#%d was ready", pconn.idx)
				return pconn, nil
			}
		default:
			break Loop
		}
	}
	for {
		c.mu.Lock()
		if c.numOpenConns() < len(c.hosts)*c.config.ConnectionsPerHost {
			c.connIdx++
			idx := c.connIdx
			var host string
			for i := idx; i < idx+len(c.hosts); i++ {
				if c.numConnsPerHost[c.hosts[i%len(c.hosts)]] < c.config.ConnectionsPerHost {
					host = c.hosts[i%len(c.hosts)]
					break
				}
			}
			if host == "" {
				panic("this shouldn't be possible")
			}
			c.numConnsPerHost[host]++
			c.mu.Unlock()
			pconn, err := c.openConn(idx, host)
			if err != nil {
				c.mu.Lock()
				c.numConnsPerHost[host]--
				c.mu.Unlock()
				c.debug("#%d error connecting: %s", idx, err)
			}
			return pconn, err
		}
		c.mu.Unlock()
		pconn := <-c.freeConnCh
		if pconn.broken {
			c.debug("waited and got #%d (broken)", pconn.idx)
			c.mu.Lock()
			c.numConnsPerHost[pconn.host]--
			c.mu.Unlock()
			c.removeConn(pconn)
		} else {
			c.debug("waited and got #%d", pconn.idx)
			return pconn, nil
		}
	}
}

func (c *Client) removeConn(pconn *persistentConn) {
	c.mu.Lock()
	delete(c.allCons, pconn.idx)
	c.mu.Unlock()
	pconn.close()
}

func (c *Client) returnConn(pconn *persistentConn) {
	c.freeConnCh <- pconn
}

func (c *Client) OpenRawConn() (RawConn, error) {
	c.mu.Lock()
	idx := c.rawConnIdx
	host := c.hosts[idx%len(c.hosts)]
	c.rawConnIdx++
	c.mu.Unlock()
	return c.openConn(-(idx + 1), host)
}

func (c *Client) openConn(idx int, host string) (pconn *persistentConn, err error) {
	pconn = &persistentConn{
		idx:              idx,
		features:         make(map[string]string),
		config:           c.config,
		t0:               c.t0,
		currentType:      "A",
		host:             host,
		epsvNotSupported: c.config.DisableEPSV,
	}
	var conn net.Conn
	if c.config.TLSConfig != nil && c.config.TLSMode == TLSImplicit {
		pconn.debug("opening TLS control connection to %s", host)
		dialer := &net.Dialer{
			Timeout: c.config.Timeout,
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", host, pconn.config.TLSConfig)
	} else {
		pconn.debug("opening control connection to %s", host)
		conn, err = net.DialTimeout("tcp", host, c.config.Timeout)
	}
	var (
		code int
		msg  string
	)
	if err != nil {
		var isTemporary bool
		if ne, ok := err.(net.Error); ok {
			isTemporary = ne.Temporary()
		}
		err = ftpError{
			err:       err,
			temporary: isTemporary,
		}
		goto Error
	}
	pconn.setControlConn(conn)
	code, msg, err = pconn.readResponse()
	if err != nil {
		goto Error
	}
	if code != replyServiceReady {
		err = ftpError{code: code, msg: msg}
		goto Error
	}
	if c.config.TLSConfig != nil && c.config.TLSMode == TLSExplicit {
		err = pconn.logInTLS()
	} else {
		err = pconn.logIn()
	}
	if err != nil {
		goto Error
	}
	if err = pconn.fetchFeatures(); err != nil {
		goto Error
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		err = ftpError{err: errors.New("client closed")}
		goto Error
	}
	if idx >= 0 {
		c.allCons[idx] = pconn
	}
	return pconn, nil
Error:
	pconn.close()
	return nil, err
}
