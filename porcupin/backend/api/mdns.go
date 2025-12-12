package api

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	// MDNSServiceType is the mDNS service type for Porcupin
	MDNSServiceType = "_porcupin._tcp"

	// MDNSDomain is the mDNS domain (trailing dot for proper DNS format)
	MDNSDomain = "local."
)

// MDNSServer handles mDNS service announcement
type MDNSServer struct {
	server   *zeroconf.Server
	port     int
	version  string
	useTLS   bool
	hostname string
}

// NewMDNSServer creates a new mDNS server for service announcement
func NewMDNSServer(port int, version string, useTLS bool) *MDNSServer {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "porcupin"
	}

	return &MDNSServer{
		port:     port,
		version:  version,
		useTLS:   useTLS,
		hostname: hostname,
	}
}

// Start begins announcing the Porcupin service via mDNS
func (m *MDNSServer) Start() error {
	// Build TXT records with metadata
	txt := []string{
		fmt.Sprintf("version=%s", m.version),
		fmt.Sprintf("tls=%v", m.useTLS),
	}

	// Create instance name (hostname-porcupin)
	instanceName := fmt.Sprintf("%s-porcupin", m.hostname)

	// Register the service
	server, err := zeroconf.Register(
		instanceName,      // Instance name
		MDNSServiceType,   // Service type
		MDNSDomain,        // Domain
		m.port,            // Port
		txt,               // TXT records
		nil,               // Interfaces (nil = all)
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}

	m.server = server
	log.Printf("mDNS: Announcing %s.%s on port %d", instanceName, MDNSServiceType, m.port)

	return nil
}

// Stop stops the mDNS service announcement
func (m *MDNSServer) Stop() {
	if m.server != nil {
		// Shutdown can block if network is unavailable, run with timeout
		done := make(chan struct{})
		go func() {
			m.server.Shutdown()
			close(done)
		}()
		
		select {
		case <-done:
			// Clean shutdown
		case <-time.After(2 * time.Second):
			log.Println("mDNS: Shutdown timed out")
		}
		m.server = nil
		log.Println("mDNS: Service announcement stopped")
	}
}

// DiscoveredServer represents a Porcupin server found via mDNS
type DiscoveredServer struct {
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Version  string   `json:"version"`
	UseTLS   bool     `json:"useTLS"`
	IPs      []string `json:"ips"`
}

// DiscoverServers scans for Porcupin servers on the local network via mDNS
// timeout specifies how long to scan (e.g., 5*time.Second)
func DiscoverServers(ctx context.Context, timeout time.Duration) ([]DiscoveredServer, error) {
	log.Printf("mDNS: Starting discovery scan for service=%q domain=%q (timeout: %v)", MDNSServiceType, MDNSDomain, timeout)
	
	// Find interfaces with IPv4 addresses for multicast
	var ipv4Ifaces []net.Interface
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		hasIPv4 := false
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
					hasIPv4 = true
					break
				}
			}
		}
		if hasIPv4 {
			ipv4Ifaces = append(ipv4Ifaces, iface)
			log.Printf("mDNS: Using interface %s for discovery", iface.Name)
		}
	}
	
	if len(ipv4Ifaces) == 0 {
		log.Printf("mDNS: WARNING - No interfaces with IPv4 addresses found!")
	}
	
	// Create resolver with IPv4 only and specific interfaces
	resolver, err := zeroconf.NewResolver(
		zeroconf.SelectIPTraffic(zeroconf.IPv4),
		zeroconf.SelectIfaces(ipv4Ifaces),
	)
	if err != nil {
		log.Printf("mDNS: Failed to create resolver: %v", err)
		return nil, fmt.Errorf("failed to create mDNS resolver: %w", err)
	}
	log.Printf("mDNS: Resolver created successfully (IPv4 only, %d interfaces)", len(ipv4Ifaces))

	// Channel to receive entries
	entries := make(chan *zeroconf.ServiceEntry)
	var servers []DiscoveredServer

	// Use done channel to signal when goroutine finishes processing
	done := make(chan struct{})

	// Start collecting entries in background
	go func() {
		defer close(done)
		for entry := range entries {
			log.Printf("mDNS: Found entry: %s at %s:%d", entry.Instance, entry.HostName, entry.Port)
			server := parseServiceEntry(entry)
			if server != nil {
				servers = append(servers, *server)
			}
		}
	}()

	// Create timeout context
	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Browse for services - this starts async browsing
	log.Printf("mDNS: Calling Browse() for %s in %s", MDNSServiceType, MDNSDomain)
	err = resolver.Browse(scanCtx, MDNSServiceType, MDNSDomain, entries)
	if err != nil {
		log.Printf("mDNS: Browse() returned error: %v", err)
		return nil, fmt.Errorf("failed to browse mDNS services: %w", err)
	}
	log.Printf("mDNS: Browse() started successfully, waiting %v for results...", timeout)

	// Wait for scan to complete
	<-scanCtx.Done()
	log.Printf("mDNS: Context done (reason: %v), waiting for goroutine to finish", scanCtx.Err())

	// Wait for goroutine to finish processing all entries
	<-done

	log.Printf("mDNS: Scan complete, found %d servers", len(servers))
	return servers, nil
}

// parseServiceEntry converts a zeroconf entry to our DiscoveredServer type
func parseServiceEntry(entry *zeroconf.ServiceEntry) *DiscoveredServer {
	if entry == nil {
		return nil
	}

	server := &DiscoveredServer{
		Name: entry.Instance,
		Host: entry.HostName,
		Port: entry.Port,
	}

	// Parse TXT records for metadata
	for _, txt := range entry.Text {
		if len(txt) > 8 && txt[:8] == "version=" {
			server.Version = txt[8:]
		}
		if txt == "tls=true" {
			server.UseTLS = true
		}
	}

	// Collect IP addresses
	for _, ip := range entry.AddrIPv4 {
		server.IPs = append(server.IPs, ip.String())
	}
	for _, ip := range entry.AddrIPv6 {
		// Filter out link-local IPv6 addresses for cleaner display
		if !ip.IsLinkLocalUnicast() {
			server.IPs = append(server.IPs, ip.String())
		}
	}

	// Prefer IPv4 for host if available
	if len(entry.AddrIPv4) > 0 {
		server.Host = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		// Find a non-link-local IPv6
		for _, ip := range entry.AddrIPv6 {
			if !ip.IsLinkLocalUnicast() {
				server.Host = ip.String()
				break
			}
		}
	}

	// Return nil if no usable IPs
	if len(server.IPs) == 0 {
		return nil
	}

	return server
}

// GetLocalIPs returns the local IP addresses of this machine
func GetLocalIPs() []string {
	var ips []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Skip loopback and link-local
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			// Prefer IPv4, but include IPv6 too
			ips = append(ips, ip.String())
		}
	}

	return ips
}
