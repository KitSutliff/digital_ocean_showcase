package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

// ResponseCode represents protocol response codes returned by the server.
// These constants match the wire protocol specification exactly for test compatibility.
type ResponseCode string

// Protocol response codes matching server specification
const (
	OK      = "OK"      // Successful command execution
	FAIL    = "FAIL"    // Command failed due to business logic constraints
	ERROR   = "ERROR"   // Protocol or parsing errors
	UNKNOWN = "UNKNOWN" // Unexpected server response
)

// PackageIndexerClient defines the interface for communicating with the package indexer server.
// This abstraction enables testing against different server implementations and connection types.
type PackageIndexerClient interface {
	Name() string
	Close() error
	Send(msg string) (ResponseCode, error)
}

// TCPPackageIndexerClient implements PackageIndexerClient using TCP connections.
// This is the production-equivalent client used for integration testing and validation.
type TCPPackageIndexerClient struct {
	name string
	conn net.Conn
}

// Name returns this client's identifier for logging and debugging purposes.
func (client *TCPPackageIndexerClient) Name() string {
	return client.name
}

// Close terminates the connection to the server and cleans up resources.
func (client *TCPPackageIndexerClient) Close() error {
	log.Printf("%s disconnecting", client.Name())
	return client.conn.Close()
}

// Send transmits a message to the server using the line-oriented protocol.
// Handles connection timeouts and protocol parsing for robust test execution.
func (client *TCPPackageIndexerClient) Send(msg string) (ResponseCode, error) {
	extendTimeoutFor(client.conn)
	_, err := fmt.Fprintln(client.conn, msg)

	if err != nil {
		return UNKNOWN, fmt.Errorf("Error sending message to server: %v", err)
	}

	extendTimeoutFor(client.conn)
	responseMsg, err := bufio.NewReader(client.conn).ReadString('\n')
	if err != nil {
		return UNKNOWN, fmt.Errorf("Error reading response code from server: %v", err)
	}

	returnedString := strings.TrimRight(responseMsg, "\n")

	if returnedString == OK {
		return OK, nil
	}

	if returnedString == FAIL {
		return FAIL, nil
	}

	if returnedString == ERROR {
		return ERROR, nil
	}

	return UNKNOWN, fmt.Errorf("Error parsing message from server [%s]: %v", responseMsg, err)
}

// MakeTCPPackageIndexClient returns a new instance of the client
func MakeTCPPackageIndexClient(name string, hostname string, port int) (PackageIndexerClient, error) {
	host := net.JoinHostPort(hostname, strconv.Itoa(port))
	log.Printf("%s connecting to [%s]", name, host)
	conn, err := net.Dial("tcp", host)

	if err != nil {
		return nil, fmt.Errorf("Failed to open connection to [%s]: %#v", host, err)
	}

	return &TCPPackageIndexerClient{
		name: name,
		conn: conn,
	}, nil
}

// extendTimeoutFor sets a 10-second deadline on the connection to prevent hangs
func extendTimeoutFor(conn net.Conn) {
	deadline := time.Now().Add(10 * time.Second)
	conn.SetDeadline(deadline)
}
