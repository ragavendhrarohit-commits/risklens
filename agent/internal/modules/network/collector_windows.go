//go:build windows

package network

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const publicIPRefreshInterval = 5 * time.Minute

var (
	publicIPMu        sync.Mutex
	cachedPublicIP    string
	publicIPCheckedAt time.Time
)

func collect(ctx context.Context) EndpointNetwork {
	hostname, _ := os.Hostname()
	return EndpointNetwork{
		PublicIP:  lookupPublicIP(ctx),
		PrivateIP: primaryPrivateIP(),
		Hostname:  strings.TrimSpace(hostname),
	}
}

func primaryPrivateIP() string {
	if ip := outboundPrivateIP(); ip != "" {
		return ip
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if !usableInterface(iface) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ip := ipv4FromAddr(addr); ip != "" {
				return ip
			}
		}
	}
	return ""
}

func outboundPrivateIP() string {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return ""
	}
	v4 := addr.IP.To4()
	if v4 == nil {
		return ""
	}
	return v4.String()
}

func usableInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	name := strings.ToLower(iface.Name)
	switch {
	case strings.Contains(name, "vethernet"),
		strings.Contains(name, "wsl"),
		strings.Contains(name, "hyper-v"),
		strings.Contains(name, "tailscale"),
		strings.Contains(name, "loopback"):
		return false
	default:
		return true
	}
}

func ipv4FromAddr(addr net.Addr) string {
	var ip net.IP
	switch typed := addr.(type) {
	case *net.IPNet:
		ip = typed.IP
	case *net.IPAddr:
		ip = typed.IP
	}
	if ip == nil || ip.IsLoopback() {
		return ""
	}
	v4 := ip.To4()
	if v4 == nil {
		return ""
	}
	return v4.String()
}

func lookupPublicIP(ctx context.Context) string {
	publicIPMu.Lock()
	if cachedPublicIP != "" && time.Since(publicIPCheckedAt) < publicIPRefreshInterval {
		ip := cachedPublicIP
		publicIPMu.Unlock()
		return ip
	}
	publicIPMu.Unlock()

	ip := fetchPublicIP(ctx)
	if ip == "" {
		publicIPMu.Lock()
		defer publicIPMu.Unlock()
		return cachedPublicIP
	}

	publicIPMu.Lock()
	cachedPublicIP = ip
	publicIPCheckedAt = time.Now()
	publicIPMu.Unlock()
	return ip
}

func fetchPublicIP(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org?format=text", nil)
	if err != nil {
		return ""
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}
