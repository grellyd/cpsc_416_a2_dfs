package addrparse

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func TCP(addr string) (net.TCPAddr, error) {
	splitAddr := strings.SplitN(addr, ":", 2)
	ip, err := parseIP(splitAddr[0])
	if err != nil {
		return net.TCPAddr{}, fmt.Errorf("address parsing failed: %s as TCP: %v", addr, err)
	}
	port, err := parsePort(splitAddr[1])
	if err != nil {
		return net.TCPAddr{}, fmt.Errorf("address parsing failed: %s as TCP: %v", addr, err)
	}
	return net.TCPAddr{IP: ip, Port: port, Zone: ""}, nil
}

func UDP(addr string) (net.UDPAddr, error) {
	splitAddr := strings.SplitN(addr, ":", 2)
	ip, err := parseIP(splitAddr[0])
	if err != nil {
		return net.UDPAddr{}, fmt.Errorf("address parsing failed: %s as UDP: %v", addr, err)
	}
	port, err := parsePort(splitAddr[1])
	if err != nil {
		return net.UDPAddr{}, fmt.Errorf("address parsing failed: %s as UDP: %v", addr, err)
	}
	return net.UDPAddr{IP: ip, Port: port, Zone: ""}, nil
}

// validates and converts a string containing a Port
func parsePort(port string) (int, error) {
	portValue, err := strconv.Atoi(port)
	if err != nil {
		return -1, fmt.Errorf("port parsing failed: %s as port: %v", port, err)
	}
	return portValue, nil
}

// validates and converts a string containing an IP
func parseIP(ip string) (net.IP, error) {
	addr := []byte{}
	for _, addrPart := range strings.Split(ip, ".") {
		parsedValue, err := strconv.Atoi(addrPart)
		if err != nil {
			return nil, fmt.Errorf("ip parsing failed: %s as ip: %v", ip, err)
		}
		addr = append(addr, byte(parsedValue))
	}
	if len(addr) != 4 {
		return nil, fmt.Errorf("ip parsing failed: %s as ip", ip)
	}
	return net.IPv4(addr[0], addr[1], addr[2], addr[3]), nil
}
