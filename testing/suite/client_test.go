package main

import (
	"fmt"
	"net"
	"os"
	"testing"
)

const (
	testInvalidHost = "nonexistent.invalid" // RFC 2606 reserved TLD ensures DNS failure
)

// respondWith accepts a single connection and sends the specified response code
func respondWith(t *testing.T, server net.Listener, responseCode string) {
	conn, err := server.Accept()
	if err != nil {
		fmt.Println("Accept failed:", err)
		os.Exit(1)
	} else {
		fmt.Fprintln(conn, responseCode)
		conn.Close()
	}
}

func TestMakeTCPPackageIndexClient(t *testing.T) {
	// Deterministically fail by using a guaranteed-invalid host
	client, err := MakeTCPPackageIndexClient("portisntopen", testInvalidHost, 12345)

	if err == nil {
		t.Errorf("Expected connection to invalid host to raise error, got %v", client)
	}
}

func TestSend(t *testing.T) {
	// Good server on an ephemeral port
	goodServer, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Error opening test server: %v", err)
	}
	defer goodServer.Close()

	go respondWith(t, goodServer, "OK")

	goodPort := goodServer.Addr().(*net.TCPAddr).Port
	client, err := MakeTCPPackageIndexClient("goodPort", "localhost", goodPort)
	if err != nil {
		t.Fatalf("Error connecting to server: %v", err)
	}

	responseCode, err := client.Send("A")

	if err != nil {
		t.Errorf("Error sending message to server: %v", err)
	}

	if responseCode != OK {
		t.Errorf("Expected responseCode to be OK, got %v", responseCode)
	}

	// Bad server on an ephemeral port that returns an unknown response
	badServer, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Error opening test server: %v", err)
	}
	defer badServer.Close()

	go respondWith(t, badServer, "banana")

	badPort := badServer.Addr().(*net.TCPAddr).Port
	client, err = MakeTCPPackageIndexClient("badPort", "localhost", badPort)
	if err != nil {
		t.Fatalf("Error connecting to server: %v", err)
	}

	responseCode, err = client.Send("B")

	if err == nil {
		t.Errorf("No error returned for bad responseCode from server: %#v", responseCode)
	}
}
