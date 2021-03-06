package httpnet

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

type TCPClient struct {
	restyClient *resty.Client
	serverURL   string
}

func NewTCPClient(serverURL string, customHttpHeader map[string]string) *TCPClient {

	log.Infof("NewTCPClient, serverURL: %s", serverURL)

	r := resty.New()
	r.SetHeaders(customHttpHeader)

	return &TCPClient{
		restyClient: r,
		serverURL:   serverURL,
	}
}

func (p *TCPClient) Dail() (conn net.Conn, err error) {
	resp, err := p.restyClient.R().
		SetHeader("action", actionHandshake).
		Post(p.serverURL)

	if err != nil {
		log.Errorf("tcp client dail failed: %v", err)
		return
	}

	if resp.StatusCode() != 200 {
		err = fmt.Errorf("tcp client dail failed, not 200")
		return
	}

	connId := resp.Header().Get("conn-id")
	if len(connId) == 0 {
		err = fmt.Errorf("connection id is nil")
		return
	}

	conn = &clientConn{
		connId:      connId,
		serverURL:   p.serverURL,
		restyClient: p.restyClient,
	}
	return
}

type clientConn struct {
	connId      string
	restyClient *resty.Client
	serverURL   string

	writeSeqId uint64
	readSeqId  uint64

	isClosed bool
}

func (p *clientConn) Read(b []byte) (n int, err error) {
	if p.isClosed {
		return 0, &net.OpError{Op: "read", Err: errors.New("connection is closed")}
	}

	var resp *resty.Response
	// max wait 30*10 seconds
	for i := 0; i < 30; i++ {
		resp, err = p.restyClient.R().
			SetHeader("action", "read").
			SetHeader("max-size", strconv.Itoa(len(b))).
			SetHeader("conn-id", p.connId).
			Post(p.serverURL)

		if err != nil {
			log.Errorf("client conn read failed: %v", err)
			return
		}

		if resp.StatusCode() != http.StatusContinue {
			break
		}

		log.Infof("client conn read with continue, connId: %s", p.connId)
	}

	if resp.StatusCode() != 200 {
		err = fmt.Errorf("tcp conn read failed, not 200")
		return
	}

	seqIdStr := resp.Header().Get("seq-id")
	if len(seqIdStr) == 0 {
		log.Warnf("client read, seq id is nil")
		return
	}

	seqId, err := strconv.ParseUint(seqIdStr, 10, 64)
	if err != nil {
		log.Warnf("invalid seq id str: %s", seqIdStr)
		return
	}

	if seqId != p.readSeqId+1 {
		err = errors.New("invalid seq id")
		log.Errorf("%v", err)
		return
	}

	log.Debugf("client read, connId: %s, seqId: %d", p.connId, seqId)

	p.readSeqId = seqId

	sizeStr := resp.Header().Get("size")
	if len(sizeStr) == 0 {
		err = errors.New("client read size header is nil")
		log.Errorf("%v", err)
		return
	}

	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		err = fmt.Errorf("client read invalid size: %v", sizeStr)
		log.Errorf("%v", err)
		return
	}

	n = copy(b, resp.Body())

	if n != size {
		log.Errorf("client read size is not equal copy size")
	}
	return
}

func (p *clientConn) Write(b []byte) (n int, err error) {
	if p.isClosed {
		return 0, &net.OpError{Op: "write", Err: errors.New("connection is closed")}
	}

	p.writeSeqId++
	resp, err := p.restyClient.R().
		SetHeader("action", actionWrite).
		SetHeader("conn-id", p.connId).
		SetHeader("seq-id", strconv.FormatUint(p.writeSeqId, 10)).
		SetHeader("size", strconv.Itoa(len(b))).
		SetBody(b).
		Post(p.serverURL)

	if err != nil {
		log.Errorf("client conn read failed: %v", err)
		return
	}

	if resp.StatusCode() != 200 {
		err = fmt.Errorf("tcp client conn write failed, not 200")
		return
	}

	writeSizeHeader := resp.Header().Get("write-size")
	if len(writeSizeHeader) == 0 {
		err = fmt.Errorf("tcp client conn write size header is nil")
		return
	}

	n, err = strconv.Atoi(writeSizeHeader)
	if err != nil {
		err = fmt.Errorf("invalid write size: %s", writeSizeHeader)
		return
	}

	if n != len(b) {
		log.Errorf("invalid client write size")
	}
	return
}

func (p *clientConn) Close() error {

	p.isClosed = true

	log.Debugf("client close connection, id: %s", p.connId)

	resp, err := p.restyClient.R().
		SetHeader("action", actionClose).
		SetHeader("conn-id", p.connId).
		Post(p.serverURL)

	if err != nil {
		log.Errorf("client conn close failed: %v", err)
		return err
	}

	if resp.StatusCode() != 200 {
		err = fmt.Errorf("tcp client conn close failed, status code: %d, resp: %s", resp.StatusCode(), resp.Body())
		return err
	}
	return nil
}

func (p *clientConn) LocalAddr() net.Addr {
	return nil
}

func (p *clientConn) RemoteAddr() net.Addr {
	return nil
}

func (p *clientConn) SetDeadline(t time.Time) error {
	return nil
}

func (p *clientConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (p *clientConn) SetWriteDeadline(t time.Time) error {
	return nil
}
