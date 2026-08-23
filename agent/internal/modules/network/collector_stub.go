//go:build !windows

package network

import (
	"context"
	"net"
	"os"
	"strings"
)

func collect(ctx context.Context) EndpointNetwork {
	_ = ctx
	hostname, _ := os.Hostname()
	return EndpointNetwork{
		PublicIP:  "",
		PrivateIP: primaryPrivateIP(),
		Hostname:  strings.TrimSpace(hostname),
	}
}

func primaryPrivateIP() string {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP != nil {
			if v4 := addr.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		var ip net.IP
		switch typed := addr.(type) {
		case *net.IPNet:
			ip = typed.IP
		case *net.IPAddr:
			ip = typed.IP
		}
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}
