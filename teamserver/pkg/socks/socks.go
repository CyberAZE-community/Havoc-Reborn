package socks

import (
	"errors"
	"net"
	"strings"
	"sync"
)

type Socks struct {
	listener net.Listener
	addr     string
	handler  func(s *Socks, conn net.Conn)
	Failed   bool

	// ClientsMtx guards Clients
	ClientsMtx sync.Mutex
	Clients    []int32
}

// ClientsAdd appends a client socket id under the Clients lock.
func (s *Socks) ClientsAdd(SocketID int32) {
	s.ClientsMtx.Lock()
	s.Clients = append(s.Clients, SocketID)
	s.ClientsMtx.Unlock()
}

// ClientsSnapshot returns a copy of the client socket ids that is safe
// to iterate while handlers append new clients.
func (s *Socks) ClientsSnapshot() []int32 {
	s.ClientsMtx.Lock()
	defer s.ClientsMtx.Unlock()

	clients := make([]int32, len(s.Clients))
	copy(clients, s.Clients)

	return clients
}

func NewSocks(addr string) *Socks {
	var socks = new(Socks)

	if !strings.Contains(addr, ":") {
		return nil
	}

	socks.addr = addr

	return socks
}

func (s *Socks) SetHandler(handler func(s *Socks, conn net.Conn)) {

	s.handler = handler

}

func (s *Socks) Start() error {
	var (
		err error
		con net.Conn
	)

	if s.handler == nil {
		return errors.New("handler not specified")
	}

	/* listen on the specified addr */
	if s.listener, err = net.Listen("tcp", s.addr); err != nil {
		return err
	}

	for {

		/* accepts any new connections */
		if con, err = s.listener.Accept(); err != nil {
			return err
		}

		go s.handler(s, con)

	}
}

func (s *Socks) Close() {

	if s.listener != nil {
		s.listener.Close()
	}

}
