package httpnet

import (
	"fmt"
	"strconv"

	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

type tcpClient struct {
	restyClient *resty.Client
	serverURL   string
}

func NewTCPClient(serverURL string) *tcpClient {
	return &tcpClient{
		restyClient: resty.New(),
		serverURL:   serverURL,
	}
}

func (p *tcpClient) Dail() (conn *clientConn, err error) {
	resp, err := p.restyClient.R().
		SetHeader("action", "handshake").
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

	log.Debugf("dail conn id: %s", connId)

	conn = &clientConn{
		id:          connId,
		serverURL:   p.serverURL,
		restyClient: p.restyClient,
	}
	return
}

type clientConn struct {
	id          string
	restyClient *resty.Client
	serverURL   string
}

func (p *clientConn) Read(b []byte) (n int, err error) {
	resp, err := p.restyClient.R().
		SetHeader("action", "read").
		SetHeader("max-size", strconv.Itoa(len(b))).
		SetHeader("conn-id", p.id).
		Post(p.serverURL)

	if err != nil {
		log.Errorf("client conn read failed: %v", err)
		return
	}

	if resp.StatusCode() != 200 {
		err = fmt.Errorf("tcp conn read failed, not 200")
		return
	}

	n = copy(b, resp.Body())
	return
}

func (p *clientConn) Write(b []byte) (n int, err error) {
	resp, err := p.restyClient.R().
		SetHeader("action", "write").
		SetHeader("conn-id", p.id).
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
	return
}

func (p *clientConn) Close() error {
	resp, err := p.restyClient.R().
		SetHeader("action", "close").
		SetHeader("conn-id", p.id).
		Post(p.serverURL)

	if err != nil {
		log.Errorf("client conn close failed: %v", err)
		return err
	}

	if resp.StatusCode() != 200 {
		err = fmt.Errorf("tcp client conn close failed, not 200")
		return err
	}
	return nil
}
