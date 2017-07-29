package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

func main() {
	var (
		mode = flag.String("mode", "server", "Whether to run as server or client")
		addr = flag.String("address", "127.0.0.1:8888", "address to connect to")
	)
	flag.Parse()

	var err error
	switch {
	case strings.ToLower(*mode) == "server":
		err = runServer(*addr)
	case strings.ToLower(*mode) == "client":
		err = runClient(*addr)
	default:
		err = errors.New("Mode must be server or client")
	}
	if err != nil {
		log.Fatalf("%s", err)
	}
}

type ConnectionPool struct {
	connections map[string]net.Conn
	nicks       map[string]string
	rnicks      map[string]string
	connectc    chan net.Conn
	disconnectc chan net.Conn
	broadcastc  chan Message
	quitc       chan struct{}
}

type Message struct {
	connection net.Conn
	message    string
}

func newConnectionPool() *ConnectionPool {
	return &ConnectionPool{
		connections: map[string]net.Conn{},
		nicks:       map[string]string{},
		rnicks:      map[string]string{},
		connectc:    make(chan net.Conn),
		disconnectc: make(chan net.Conn),
		broadcastc:  make(chan Message),
		quitc:       make(chan struct{}),
	}
}

func (pool *ConnectionPool) Add(conn net.Conn) {
	pool.connectc <- conn
}

func (pool *ConnectionPool) Delete(conn net.Conn) {
	pool.disconnectc <- conn
}

func (pool *ConnectionPool) Broadcast(conn net.Conn, message string) {
	pool.broadcastc <- Message{connection: conn, message: message}
}

func (pool *ConnectionPool) Shutdown() {
	close(pool.quitc)
}

func (pool *ConnectionPool) Run() {
	for {
		select {
		case conn := <-pool.connectc:
			name := conn.RemoteAddr().String()
			log.Printf("Client %s: connected", name)
			pool.connections[name] = conn
		case conn := <-pool.disconnectc:
			name := conn.RemoteAddr().String()
			log.Printf("Client %s: disconnected", name)
			delete(pool.connections, name)
		case message := <-pool.broadcastc:
			pool.handleMessage(message)
		case <-pool.quitc:
			for _, conn := range pool.connections {
				conn.Close()
			}
			return
		}
	}

}

func (pool *ConnectionPool) handleMessage(message Message) {
	name := message.connection.RemoteAddr().String()
	nick := pool.nicks[name]
	if nick == "" {
		nick = name
	}
	text := message.message
	log.Printf("Client %s: \"%s\"", name, text)

	// Allow setting and changing of nicknames
	if strings.HasPrefix(text, "/nick ") {
		// clean off the command from the start of the message
		newNick := strings.TrimPrefix(text, "/nick ")

		// nicks must be one word
		if strings.Index(nick, " ") != -1 {
			fmt.Fprintf(message.connection, "Nickname '%s' cannot have spaces in\n", newNick)
			return
		}

		// check to see if the nickname is already taken - if it is tell the requester and the owner
		if remote := pool.connectionFromNickOrName(newNick); remote != nil {
			fmt.Fprintf(message.connection, "Nickname '%s' is already taken by %s\n", newNick, remote.RemoteAddr().String())
			fmt.Fprintf(remote, "User '%s' tried to steal your nickname '%s'\n", name, newNick)
			return
		}

		pool.nicks[name] = newNick
		pool.rnicks[newNick] = message.connection.RemoteAddr().String()
		log.Printf("%s has changed their nickname from '%s' to '%s'", name, nick, newNick)
		text = fmt.Sprintf("Nickname changed to '%s'", newNick)
	}

	// Allow sending private messages
	if strings.HasPrefix(text, "/privmsg") {
		// clean off the command from the start of the message
		text = strings.TrimPrefix(text, "/privmsg ")
		// now get the recipient and the message
		parts := strings.SplitN(text, " ", 2)
		rnick, text := parts[0], parts[1]

		remote := pool.connectionFromNickOrName(rnick)
		if remote == nil {
			fmt.Fprintf(message.connection, "Couldn't find a person named '%s'", rnick)
		} else if remote == message.connection {
			fmt.Fprintf(message.connection, "You can't privmsg yourself")
		} else {
			fmt.Fprintf(remote, "%s (private)> %s\n", nick, text)
			fmt.Fprintf(message.connection, "Message sent\n")
		}
		return
	}

	for n, c := range pool.connections {
		if n == name {
			fmt.Fprintf(c, "> %s\n", text)
		} else {
			fmt.Fprintf(c, "%s> %s\n", nick, text)
		}
	}
}

func (pool *ConnectionPool) connectionFromNickOrName(name string) net.Conn {
	if tmp, ok := pool.rnicks[name]; ok {
		name = tmp
	}
	return pool.connections[name]
}

func runServer(address string) error {
	var err error
	var pool = newConnectionPool()
	defer pool.Shutdown()
	go pool.Run()

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Printf("Listening on %s", address)

	for {
		conn, err := ln.Accept()
		if err != nil {
			break
		}
		go func() {
			pool.Add(conn)
			defer pool.Delete(conn)
			scanner := bufio.NewScanner(conn)
			for scanner.Scan() {
				text := scanner.Text()
				pool.Broadcast(conn, text)
			}
		}()
	}

	return err
}

func runClient(address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	log.Printf("Connected to %s", address)
	errc := make(chan error, 2)

	go func() {
		s := bufio.NewScanner(conn)
		for s.Scan() {
			fmt.Printf("%s\n", s.Text())
		}
		errc <- s.Err()
	}()

	go func() {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			text := s.Text()
			if text == "quit" {
				break
			}
			fmt.Fprintf(conn, "%s\n", text)
		}
		errc <- s.Err()
	}()
	return <-errc
}
