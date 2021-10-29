package gossip

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/go-sockaddr"
	"github.com/hashicorp/memberlist"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type messageType uint8

const (
	_ messageType = iota // don't use 0
	packet
	stream
)

const zeroZeroZeroZero = "0.0.0.0"

type TCPTransportConfig struct {
	BindAddrs          []string
	BindPort           int
	PacketDialTimeout  time.Duration
	PacketWriteTimeout time.Duration
	TransportDebug     bool
}

// Copied and edited from distributed systems kit from grafana
// https://github.com/grafana/dskit/blob/main/kv/memberlist/tcp_transport.go
// TCPTransport is a memberlist.Transport implementation that uses TCP for both packet and stream
// operations ("packet" and "stream" are terms used by memberlist).
// It uses a new TCP connections for each operation. There is no connection reuse.
type TCPTransport struct {
	cfg          TCPTransportConfig
	logger       *zap.Logger
	packetCh     chan *memberlist.Packet
	connCh       chan net.Conn
	wg           sync.WaitGroup
	tcpListeners []net.Listener

	shutdown atomic.Int32

	advertiseMu   sync.RWMutex
	advertiseAddr string
}

// NewTCPTransport returns a new tcp-based transport with the given configuration. On
// success all the network listeners will be created and listening.
func NewTCPTransport(config TCPTransportConfig, logger *zap.Logger) (*TCPTransport, error) {
	if len(config.BindAddrs) == 0 {
		config.BindAddrs = []string{zeroZeroZeroZero}
	}

	// Build out the new transport.
	var ok bool
	t := TCPTransport{
		cfg:      config,
		logger:   logger,
		packetCh: make(chan *memberlist.Packet),
		connCh:   make(chan net.Conn),
	}

	var err error
	// Clean up listeners if there's an error.
	defer func() {
		if !ok {
			_ = t.Shutdown()
		}
	}()

	// Build all the TCP and UDP listeners.
	port := config.BindPort
	for _, addr := range config.BindAddrs {
		ip := net.ParseIP(addr)

		tcpAddr := &net.TCPAddr{IP: ip, Port: port}

		var tcpLn net.Listener
		tcpLn, err = net.ListenTCP("tcp", tcpAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start TCP listener on %q port %d", addr, port)
		}

		t.tcpListeners = append(t.tcpListeners, tcpLn)

		// If the config port given was zero, use the first TCP listener
		// to pick an available port and then apply that to everything
		// else.
		if port == 0 {
			port = tcpLn.Addr().(*net.TCPAddr).Port
		}
	}

	// Fire them up now that we've been able to create them all.
	for i := 0; i < len(config.BindAddrs); i++ {
		t.wg.Add(1)
		go t.tcpListen(t.tcpListeners[i])
	}

	ok = true
	return &t, nil
}

// tcpListen is a long running goroutine that accepts incoming TCP connections
// and spawns new go routine to handle each connection. This transport uses TCP connections
// for both packet sending and streams.
// (copied from Memberlist net_transport.go)
func (t *TCPTransport) tcpListen(tcpLn net.Listener) {
	defer t.wg.Done()

	// baseDelay is the initial delay after an AcceptTCP() error before attempting again
	const baseDelay = 5 * time.Millisecond

	// maxDelay is the maximum delay after an AcceptTCP() error before attempting again.
	// In the case that tcpListen() is error-looping, it will delay the shutdown check.
	// Therefore, changes to maxDelay may have an effect on the latency of shutdown.
	const maxDelay = 1 * time.Second

	var loopDelay time.Duration
	for {
		conn, err := tcpLn.Accept()
		if err != nil {
			if s := t.shutdown.Load(); s == 1 {
				break
			}

			if loopDelay == 0 {
				loopDelay = baseDelay
			} else {
				loopDelay *= 2
			}

			if loopDelay > maxDelay {
				loopDelay = maxDelay
			}

			t.logger.Error("TCPTransport: Error accepting TCP connection", zap.Error(err))
			time.Sleep(loopDelay)
			continue
		}
		// No error, reset loop delay
		loopDelay = 0

		go t.handleConnection(conn)
	}
}

func (t *TCPTransport) handleConnection(conn net.Conn) {
	t.logger.Debug("TCPTransport: New connection", zap.String("addr", conn.RemoteAddr().String()))

	closeConn := true
	defer func() {
		if closeConn {

			t.logger.Debug("Closing TCP connection")
			err := conn.Close()
			if err != nil {
				t.logger.Error("Error closing TCP connection", zap.Error(err))

			}
		}
	}()

	// let's read first byte, and determine what to do about this connection
	msgType := []byte{0}
	_, err := io.ReadFull(conn, msgType)
	if err != nil {
		t.logger.Error("TCPTransport: failed to read message type", zap.Error(err))
		return
	}

	if messageType(msgType[0]) == stream {
		// hand over this connection to memberlist
		closeConn = false
		t.connCh <- conn
	} else if messageType(msgType[0]) == packet {
		// it's a memberlist "packet", which contains an address and data.
		// before reading packet, read the address
		b := []byte{0}
		_, err := io.ReadFull(conn, b)
		if err != nil {
			t.logger.Error("TCPTransport: error while reading address:", zap.Error(err))
			return
		}

		addrBuf := make([]byte, b[0])
		_, err = io.ReadFull(conn, addrBuf)
		if err != nil {
			t.logger.Error("TCPTransport: error while reading address:", zap.Error(err))
			return
		}

		// read the rest to buffer -- this is the "packet" itself
		buf, err := ioutil.ReadAll(conn)
		if err != nil {
			t.logger.Error("TCPTransport: error while reading packet data:", zap.Error(err))
			return
		}

		if len(buf) < md5.Size {
			t.logger.Error("TCPTransport: not enough data received", zap.Int("length", len(buf)))
			return
		}

		receivedDigest := buf[len(buf)-md5.Size:]
		buf = buf[:len(buf)-md5.Size]

		expectedDigest := md5.Sum(buf)

		if !bytes.Equal(receivedDigest, expectedDigest[:]) {
			t.logger.Warn("TCPTransport: packet digest mismatch", zap.String("expected", fmt.Sprintf("%x", expectedDigest)), zap.String("received", fmt.Sprintf("%x", receivedDigest)))
		}

		t.logger.Debug("TCPTransport: Received packet", zap.String("addr", string(addrBuf)), zap.Int("size", len(buf)), zap.String("hash", fmt.Sprintf("%x", receivedDigest)))

		t.packetCh <- &memberlist.Packet{
			Buf:       buf,
			From:      addr(addrBuf),
			Timestamp: time.Now(),
		}
	} else {
		t.logger.Error("TCPTransport: unknown message type", zap.String("msgType", string(msgType)))
	}
}

type addr string

func (a addr) Network() string {
	return "tcp"
}

func (a addr) String() string {
	return string(a)
}

func (t *TCPTransport) getConnection(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

// GetAutoBindPort returns the bind port that was automatically given by the
// kernel, if a bind port of 0 was given.
func (t *TCPTransport) GetAutoBindPort() int {
	// We made sure there's at least one TCP listener, and that one's
	// port was applied to all the others for the dynamic bind case.
	return t.tcpListeners[0].Addr().(*net.TCPAddr).Port
}

// FinalAdvertiseAddr is given the user's configured values (which
// might be empty) and returns the desired IP and port to advertise to
// the rest of the cluster.
// (Copied from memberlist' net_transport.go)
func (t *TCPTransport) FinalAdvertiseAddr(ip string, port int) (net.IP, int, error) {
	var advertiseAddr net.IP
	var advertisePort int
	if ip != "" {
		// If they've supplied an address, use that.
		advertiseAddr = net.ParseIP(ip)
		if advertiseAddr == nil {
			return nil, 0, fmt.Errorf("failed to parse advertise address %q", ip)
		}

		// Ensure IPv4 conversion if necessary.
		if ip4 := advertiseAddr.To4(); ip4 != nil {
			advertiseAddr = ip4
		}
		advertisePort = port
	} else {
		if t.cfg.BindAddrs[0] == zeroZeroZeroZero {
			// Otherwise, if we're not bound to a specific IP, let's
			// use a suitable private IP address.
			var err error
			ip, err = sockaddr.GetPrivateIP()
			if err != nil {
				return nil, 0, fmt.Errorf("failed to get interface addresses: %v", err)
			}
			if ip == "" {
				return nil, 0, fmt.Errorf("no private IP address found, and explicit IP not provided")
			}

			advertiseAddr = net.ParseIP(ip)
			if advertiseAddr == nil {
				return nil, 0, fmt.Errorf("failed to parse advertise address: %q", ip)
			}
		} else {
			// Use the IP that we're bound to, based on the first
			// TCP listener, which we already ensure is there.
			advertiseAddr = t.tcpListeners[0].Addr().(*net.TCPAddr).IP
		}

		// Use the port we are bound to.
		advertisePort = t.GetAutoBindPort()
	}

	t.logger.Debug("FinalAdvertiseAddr", zap.String("advertiseAddr", advertiseAddr.String()), zap.Int("advertisePort", advertisePort))

	t.setAdvertisedAddr(advertiseAddr, advertisePort)
	return advertiseAddr, advertisePort, nil
}

func (t *TCPTransport) setAdvertisedAddr(advertiseAddr net.IP, advertisePort int) {
	t.advertiseMu.Lock()
	defer t.advertiseMu.Unlock()
	addr := net.TCPAddr{IP: advertiseAddr, Port: advertisePort}
	t.advertiseAddr = addr.String()
}

func (t *TCPTransport) getAdvertisedAddr() string {
	t.advertiseMu.RLock()
	defer t.advertiseMu.RUnlock()
	return t.advertiseAddr
}

// WriteTo is a packet-oriented interface that fires off the given
// payload to the given address.
func (t *TCPTransport) WriteTo(b []byte, addr string) (time.Time, error) {
	err := t.writeTo(b, addr)
	if err != nil {
		t.logger.Warn("TCPTransport: WriteTo failed", zap.String("addr", string(addr)), zap.Error(err))
		// WriteTo is used to send "UDP" packets. Since we use TCP, we can detect more errors,
		// but memberlist library doesn't seem to cope with that very well. That is why we return nil instead.
		return time.Now(), nil
	}

	return time.Now(), nil
}

func (t *TCPTransport) writeTo(b []byte, addr string) error {
	// Open connection, write packet header and data, data hash, close. Simple.
	c, err := t.getConnection(addr, t.cfg.PacketDialTimeout)
	if err != nil {
		return err
	}

	closed := false
	defer func() {
		if !closed {
			// If we still need to close, then there was another error. Ignore this one.
			_ = c.Close()
		}
	}()

	if t.cfg.PacketWriteTimeout > 0 {
		deadline := time.Now().Add(t.cfg.PacketWriteTimeout)
		err := c.SetDeadline(deadline)
		if err != nil {
			return fmt.Errorf("setting deadline: %v", err)
		}
	}

	buf := bytes.Buffer{}
	buf.WriteByte(byte(packet))

	// We need to send our address to the other side, otherwise other side can only see IP and port from TCP header.
	// But that doesn't match our node address (new TCP connection has new random port), which confuses memberlist.
	// So we send our advertised address, so that memberlist on the receiving side can match it with correct node.
	// This seems to be important for node probes (pings) done by memberlist.
	ourAddr := t.getAdvertisedAddr()
	if len(ourAddr) > 255 {
		return fmt.Errorf("local address too long")
	}

	buf.WriteByte(byte(len(ourAddr)))
	buf.WriteString(ourAddr)

	_, err = c.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("sending local address: %v", err)
	}

	n, err := c.Write(b)
	if err != nil {
		return fmt.Errorf("sending data: %v", err)
	}
	if n != len(b) {
		return fmt.Errorf("sending data: short write")
	}

	// Append digest. We use md5 as quick and relatively short hash, not in cryptographic context.
	// This helped to find some bugs, so let's keep it.
	digest := md5.Sum(b)
	n, err = c.Write(digest[:])
	if err != nil {
		return fmt.Errorf("digest: %v", err)
	}
	if n != len(digest) {
		return fmt.Errorf("digest: short write")
	}

	closed = true
	err = c.Close()
	if err != nil {
		return fmt.Errorf("close: %v", err)
	}

	t.logger.Debug("WriteTo: packet sent", zap.String("addr", string(addr)), zap.Int("size", len(b)), zap.String("hash", fmt.Sprintf("%x", digest)))
	return nil
}

// PacketCh returns a channel that can be read to receive incoming
// packets from other peers.
func (t *TCPTransport) PacketCh() <-chan *memberlist.Packet {
	return t.packetCh
}

// DialTimeout is used to create a connection that allows memberlist to perform
// two-way communication with a peer.
func (t *TCPTransport) DialTimeout(addr string, timeout time.Duration) (net.Conn, error) {
	c, err := t.getConnection(addr, timeout)

	if err != nil {
		return nil, err
	}

	_, err = c.Write([]byte{byte(stream)})
	if err != nil {
		_ = c.Close()
		return nil, err
	}

	return c, nil
}

// StreamCh returns a channel that can be read to handle incoming stream
// connections from other peers.
func (t *TCPTransport) StreamCh() <-chan net.Conn {
	return t.connCh
}

// Shutdown is called when memberlist is shutting down; this gives the
// transport a chance to clean up any listeners.
func (t *TCPTransport) Shutdown() error {
	// This will avoid log spam about errors when we shut down.
	t.shutdown.Store(1)

	// Rip through all the connections and shut them down.
	for _, conn := range t.tcpListeners {
		_ = conn.Close()
	}

	// Block until all the listener threads have died.
	t.wg.Wait()
	return nil
}
