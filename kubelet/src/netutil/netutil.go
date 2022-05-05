package netutil

import (
	"fmt"
	"net"
)

// GetAvailablePort returns a random available TCP port
func GetAvailablePort() (int, error) {
	address, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:0", "0.0.0.0"))
	if err != nil {
		return 0, err
	}

	listener, err := net.ListenTCP("tcp", address)
	if err != nil {
		return 0, err
	}

	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// IsPortAvailable judges whether given port is available
func IsPortAvailable(port int) bool {
	address := fmt.Sprintf("%s:%d", "0.0.0.0", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Printf("port %s is taken: %s\n", address, err)
		return false
	}

	defer listener.Close()
	return true
}