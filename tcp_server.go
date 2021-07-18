package httpnet

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/bwmarrin/snowflake"
)

const (
	actionHandshake = "handshake"
	actionRead      = "read"
	actionWrite     = "write"
	actionClose     = "close"
)

type tcpServer struct {
	addr     string
	sfNode   *snowflake.Node
	connChan chan *serverConn

	rw sync.RWMutex
	m  map[string]*serverConn
}

func NewTcpServer(addr string) *tcpServer {
	sfNode, _ := snowflake.NewNode(1)
	return &tcpServer{
		sfNode:   sfNode,
		addr:     addr,
		connChan: make(chan *serverConn, 10),
		m:        make(map[string]*serverConn),
	}
}

func (p *tcpServer) Run() (err error) {
	srv := &http.Server{
		Addr:    p.addr,
		Handler: p,
	}
	return srv.ListenAndServe()
}

func (p *tcpServer) getConn(connId string) (*serverConn, bool) {
	p.rw.RLock()
	conn, ok := p.m[connId]
	p.rw.RUnlock()
	return conn, ok
}

func (p *tcpServer) setConn(connId string, conn *serverConn) {
	p.rw.Lock()
	p.m[connId] = conn
	p.rw.Unlock()
}

func (p *tcpServer) deleteConn(connId string) {
	p.rw.Lock()
	delete(p.m, connId)
	p.rw.Unlock()
}

func (p *tcpServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	action := req.Header.Get("action")
	fmt.Println("action:", action)

	if action == actionHandshake {
		p.handleHandshake(rw, req)
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

func (p *tcpServer) handleHandshake(rw http.ResponseWriter, req *http.Request) {
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
}

func (p *tcpServer) Accept() (conn *serverConn, err error) {
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
