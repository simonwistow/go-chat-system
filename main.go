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
		mode = flag.String("mode", "server", "Whether to run as server or client (default server)")
		addr = flag.String("address", "127.0.0.1:8888", "address to connect to (default 127.1:8888)")
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

func runServer(address string) error {
	var connections = map[string]net.Conn{}
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
	}()

	ln, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	log.Printf("Listening on %s", address)

	for {
		conn, err := ln.Accept()
		if err != nil {
			break
		}
		go func() {
			name := conn.RemoteAddr().String()
			log.Printf("Client %s: connected", name)
			defer log.Printf("Client %s: disconnected", name)
			connections[name] = conn
			defer delete(connections, name)
			handle(name, conn, connections)
		}()
	}

	return err
}

func handle(name string, conn net.Conn, connections map[string]net.Conn) {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		text := scanner.Text()
		log.Printf("Client %s: \"%s\"", name, text)
		for n, c := range connections {
			if n == name {
				fmt.Fprintf(c, "> %s\n", text)
			} else {
				fmt.Fprintf(c, "%s> %s\n", name, text)
			}
		}
	}
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
