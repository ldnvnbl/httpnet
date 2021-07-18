package main

import (
	"fmt"
	"time"

	"github.com/ldnvnbl/httpnet"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetLevel(log.DebugLevel)
	fmt.Println("hello world")

	tcpServer := httpnet.NewTcpServer("127.0.0.1:9900")
	go func() {
		fmt.Println("tcp server run")
		err := tcpServer.Run()
		if err != nil {
			return
		}
	}()

	go func() {
		serverConn, _ := tcpServer.Accept()
		b := make([]byte, 500)
		for {
			log.Infof("waiting for read.....")
			_, _ = serverConn.Read(b)
			log.Infof("server read: %s", b)

			s := "Hello " + string(b)
			_, _ = serverConn.Write([]byte(s))
		}

	}()

	time.Sleep(time.Second)

	tcpClient := httpnet.NewTCPClient("http://127.0.0.1:9900")
	clientConn, err := tcpClient.Dail()
	if err != nil {
		fmt.Println("tcpClient.Dail err:", err)
		return
	}

	fmt.Println("dail success...")

	buf := make([]byte, 500)
	for {
		time.Sleep(time.Second * 2)
		clientConn.Write([]byte("world!!!!"))
		clientConn.Read(buf)
		log.Infof("client read: %s", buf)
	}

	_ = clientConn
	time.Sleep(time.Hour)
}
