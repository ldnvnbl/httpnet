package httpnet

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
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

	log.Infof("setConn, connId: %s", connId)

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
		log.Errorf("invalid action: %s", action)
		rw.WriteHeader(http.StatusBadRequest)
	}
}

func (p *TCPServer) HandleHandshake(rw http.ResponseWriter, req *http.Request) net.Conn {
	action := req.Header.Get("action")
	if action != actionHandshake {
		return nil
	}
	log.Infof("tcp server handle handshake")
	connId := p.sfNode.Generate().String()
	conn := &serverConn{
		connId:    connId,
		readChan:  make(chan []byte, 100),
		writeChan: make(chan []byte, 100),
	}
	p.setConn(connId, conn)
	//p.connChan <- conn
	rw.Header().Set("conn-id", connId)
	rw.WriteHeader(http.StatusOK)
	return conn
}

func (p *TCPServer) Accept() (conn net.Conn, err error) {
	conn = <-p.connChan
	return
}

type serverConn struct {
	connId string

	readChan  chan []byte // client write to this chan
	writeChan chan []byte // client read from this chan

	writeSeqId uint64
	readSeqId  uint64
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
	data := <-p.writeChan
	p.writeSeqId++
	rw.WriteHeader(http.StatusOK)
	rw.Header().Set("seq-id", strconv.FormatUint(p.writeSeqId, 10))
	rw.Header().Set("size", strconv.Itoa(len(data)))
	_, err := rw.Write(data)
	if err != nil {
		log.Errorf("write data to client failed: %v", err)
		return
	}
}

func (p *serverConn) handleClientWrite(rw http.ResponseWriter, req *http.Request) {
	sizeStr := req.Header.Get("size")
	if len(sizeStr) == 0 {
		rw.WriteHeader(http.StatusBadRequest)
		log.Warnf("handle client write size is nil")
		return
	}

	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		log.Warnf("handle client size str is invalid: %s", sizeStr)
		return
	}

	seqIdStr := req.Header.Get("seq-id")
	if len(seqIdStr) == 0 {
		rw.WriteHeader(http.StatusBadRequest)
		log.Warnf("handle client write seq id is nil")
		return
	}

	seqId, err := strconv.ParseUint(seqIdStr, 10, 64)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		log.Warnf("invalid seq id str: %s", seqIdStr)
		return
	}

	if seqId != p.readSeqId+1 {
		rw.WriteHeader(http.StatusBadRequest)
		log.Errorf("invalid sequence id")
		return
	}

	p.readSeqId = seqId

	log.Infof("handle client write, connId: %s, seqId: %d", p.connId, seqId)

	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Errorf("read data from client failed: %v", err)
		return
	}
	p.readChan <- data

	if size != len(data) {
		rw.WriteHeader(http.StatusBadRequest)
		log.Errorf("handle client write, data size is not equal header size")
		return
	}

	rw.WriteHeader(http.StatusOK)
	rw.Header().Set("write-size", strconv.Itoa(len(data)))
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
