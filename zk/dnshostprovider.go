package zk

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// lookupInterval is the interval of retrying DNS lookup for unresolved hosts
const lookupInterval = time.Minute * 3

// DNSHostProvider is the default HostProvider. It currently matches
// the Java StaticHostProvider, resolving hosts from DNS once during
// the call to Init.  It could be easily extended to re-query DNS
// periodically or if there is trouble connecting.
type DNSHostProvider struct {
	// fields above mu are not thread safe
	unresolvedServers map[string]struct{}
	sleep             func(time.Duration)            // Override of time.Sleep, for testing.
	lookupHost        func(string) ([]string, error) // Override of net.LookupHost, for testing.

	mu      sync.Mutex // Protects everything below, so we can add asynchronous updates later.
	servers []string
	curr    int
	last    int
}

// Init is called first, with the servers specified in the connection
// string. It uses DNS to look up addresses for each server, then
// shuffles them all together.
func (hp *DNSHostProvider) Init(servers []string) error {
	if hp.sleep == nil {
		hp.sleep = time.Sleep
	}
	if hp.lookupHost == nil {
		hp.lookupHost = net.LookupHost
	}

	hp.servers = make([]string, 0, len(servers))
	hp.unresolvedServers = make(map[string]struct{}, len(servers))
	for _, server := range servers {
		hp.unresolvedServers[server] = struct{}{}
	}

	done, err := hp.lookupUnresolvedServers()
	if err != nil {
		return err
	}

	// as long as any host resolved successfully, consider the connection as success
	// but start a lookup loop until all servers are resolved and added to servers list
	if !done {
		go hp.lookupLoop()
	}

	return nil
}

// lookupLoop calls lookupUnresolvedServers in an infinite loop until all hosts are resolved
// should be called in a separate goroutine
func (hp *DNSHostProvider) lookupLoop() {
	for {
		if done, _ := hp.lookupUnresolvedServers(); done {
			break
		}
		hp.sleep(lookupInterval)
	}
}

// lookupUnresolvedServers DNS lookup the hosts that not successfully resolved yet
// and add them to servers list
func (hp *DNSHostProvider) lookupUnresolvedServers() (bool, error) {
	if len(hp.unresolvedServers) == 0 {
		return true, nil
	}

	found := make([]string, 0, len(hp.unresolvedServers))
	for server := range hp.unresolvedServers {
		host, port, err := net.SplitHostPort(server)
		if err != nil {
			return false, err
		}
		addrs, err := hp.lookupHost(host)
		if err != nil {
			continue
		}
		delete(hp.unresolvedServers, server)
		for _, addr := range addrs {
			found = append(found, net.JoinHostPort(addr, port))
		}
	}
	// Randomize the order of the servers to avoid creating hotspots
	stringShuffle(found)

	hp.mu.Lock()
	defer hp.mu.Unlock()

	hp.servers = append(hp.servers, found...)
	hp.curr = -1
	hp.last = -1

	if len(hp.servers) == 0 {
		return true, fmt.Errorf("No hosts found for addresses %q", hp.servers)
	}

	return false, nil
}

// Len returns the number of servers available
func (hp *DNSHostProvider) Len() int {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	return len(hp.servers)
}

// Next returns the next server to connect to. retryStart will be true
// if we've looped through all known servers without Connected() being
// called.
func (hp *DNSHostProvider) Next() (server string, retryStart bool) {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	hp.curr = (hp.curr + 1) % len(hp.servers)
	retryStart = hp.curr == hp.last
	if hp.last == -1 {
		hp.last = 0
	}
	return hp.servers[hp.curr], retryStart
}

// Connected notifies the HostProvider of a successful connection.
func (hp *DNSHostProvider) Connected() {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	hp.last = hp.curr
}
