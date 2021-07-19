package httpnet

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bwmarrin/snowflake"
	log "github.com/sirupsen/logrus"
)

const (
	actionHandshake = "handshake"
	actionRead      = "read"
	actionWrite     = "write"
	actionClose     = "close"
)

type TCPServer struct {
	addr     string
	sfNode   *snowflake.Node
	connChan chan *serverConn

	rw sync.RWMutex
	m  map[string]*serverConn
}

func NewTcpServer(addr string) *TCPServer {
	sfNode, _ := snowflake.NewNode(1)
	return &TCPServer{
		sfNode:   sfNode,
		addr:     addr,
		connChan: make(chan *serverConn, 10),
		m:        make(map[string]*serverConn),
	}
}

func (p *TCPServer) Run() (err error) {
	srv := &http.Server{
		Addr:    p.addr,
		Handler: p,
	}
	return srv.ListenAndServe()
}

func (p *TCPServer) getConn(connId string) (*serverConn, bool) {
	p.rw.RLock()
	conn, ok := p.m[connId]
	p.rw.RUnlock()
	return conn, ok
}

func (p *TCPServer) setConn(connId string, conn *serverConn) {
	p.rw.Lock()
	p.m[connId] = conn
	p.rw.Unlock()
}

func (p *TCPServer) deleteConn(connId string) {
	p.rw.Lock()
	delete(p.m, connId)
	p.rw.Unlock()
}

func (p *TCPServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	action := req.Header.Get("action")
	fmt.Println("action:", action)

	if action == actionHandshake {
		p.HandleHandshake(rw, req)
		return
	}

	connId := req.Header.Get("conn-id")
	conn, ok := p.getConn(connId)
	if !ok {
		rw.WriteHeader(http.StatusBadRequest)
		log.Warnf("can't get conn by conn id: %s", connId)
		return
	}

	switch action {
	case actionWrite:
		conn.handleClientWrite(rw, req)
	case actionRead:
		conn.handleClientRead(rw, req)
	case actionClose:
		p.deleteConn(connId)
	default:

	}
}

func (p *TCPServer) HandleHandshake(rw http.ResponseWriter, req *http.Request) net.Conn {
	action := req.Header.Get("action")
	if action != actionHandshake {
		return nil
	}

	connId := p.sfNode.Generate().String()
	conn := &serverConn{
		id:        connId,
		readChan:  make(chan []byte, 100),
		writeChan: make(chan []byte, 100),
	}
	p.setConn(connId, conn)
	p.connChan <- conn
	rw.Header().Set("conn-id", connId)
	rw.WriteHeader(http.StatusOK)
	return conn
}

func (p *TCPServer) Accept() (conn net.Conn, err error) {
	conn = <-p.connChan
	return
}

type serverConn struct {
	id string

	readChan  chan []byte // client write to this chan
	writeChan chan []byte // client read from this chan
}

func (p *serverConn) Read(b []byte) (n int, err error) {
	data := <-p.readChan
	if len(data) > len(b) {
		err = fmt.Errorf("read too large size data")
		log.Error(err)
		return
	}
	n = copy(b, data)
	return
}

func (p *serverConn) Write(b []byte) (n int, err error) {
	p.writeChan <- b
	n = len(b)
	return
}

func (p *serverConn) Close() error {
	return nil
}

func (p *serverConn) handleClientRead(rw http.ResponseWriter, req *http.Request) {
	data := <- p.writeChan
	rw.WriteHeader(http.StatusOK)
	_, err := rw.Write(data)
	if err != nil {
		log.Errorf("write data to client failed: %v", err)
		return
	}
}

func (p *serverConn) handleClientWrite(rw http.ResponseWriter, req *http.Request) {
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Errorf("read data from client failed: %v", err)
		return
	}
	p.readChan <- data
}

func (p *serverConn) LocalAddr() net.Addr {
	return nil
}

func (p *serverConn) RemoteAddr() net.Addr {
	return nil
}

func (p *serverConn) SetDeadline(t time.Time) error {
	return nil
}

func (p *serverConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (p *serverConn) SetWriteDeadline(t time.Time) error {
	return nil
}
