package netpolicy

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// PinnedIPSet manages domain to IP resolution with background TTL-aware refresh.
type PinnedIPSet struct {
	mu       sync.RWMutex
	pins     map[string][]net.IP
	ttls     map[string]time.Duration
	refresh  map[string]time.Time
	resolver *net.Resolver
	cancel   context.CancelFunc
}

// LookupAndPinDomains resolves domains initially and starts background refresh.
func LookupAndPinDomains(domains []string) (*PinnedIPSet, error) {
	set := &PinnedIPSet{
		pins:     make(map[string][]net.IP),
		ttls:     make(map[string]time.Duration),
		refresh:  make(map[string]time.Time),
		resolver: net.DefaultResolver,
	}

	for _, d := range domains {
		// If wildcard domain (e.g. *.example.com), resolve base domain (example.com)
		lookupTarget := d
		isWildcard := false
		if strings.HasPrefix(d, "*.") {
			lookupTarget = d[2:]
			isWildcard = true
		}

		ips, ttl, err := lookupWithTTL(set.resolver, lookupTarget)
		if err != nil {
			if isWildcard {
				return nil, fmt.Errorf("DNS lookup failed for wildcard apex %q: %w (v0.4 requires a resolvable apex for wildcard pins; enumerate subdomains explicitly or check DNS configuration)", d, err)
			}
			return nil, fmt.Errorf("DNS lookup failed for %q: %w (network backend cannot be initialized without domain pins)", d, err)
		}
		set.pins[d] = ips
		set.ttls[d] = ttl
		set.refresh[d] = time.Now().Add(ttl)
	}

	ctx, cancel := context.WithCancel(context.Background())
	set.cancel = cancel
	go set.refreshLoop(ctx)

	return set, nil
}

func (s *PinnedIPSet) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for d, deadline := range s.refresh {
				if now.After(deadline) {
					lookupTarget := d
					if strings.HasPrefix(d, "*.") {
						lookupTarget = d[2:]
					}
					if ips, ttl, err := lookupWithTTL(s.resolver, lookupTarget); err == nil && len(ips) > 0 {
						s.pins[d] = ips
						s.ttls[d] = ttl
						s.refresh[d] = now.Add(ttl)
					} else {
						// On error, retain stale pins and retry after short interval
						s.refresh[d] = now.Add(1 * time.Minute)
					}
				}
			}
			s.mu.Unlock()
		}
	}
}

// Get returns a defensive copy of currently pinned IPs for domain.
func (s *PinnedIPSet) Get(domain string) []net.IP {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ips, ok := s.pins[domain]
	if !ok {
		return nil
	}
	out := make([]net.IP, len(ips))
	copy(out, ips)
	return out
}

// ToMap returns a string map of domain to IP strings.
func (s *PinnedIPSet) ToMap() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]string, len(s.pins))
	for d, ips := range s.pins {
		strIPs := make([]string, len(ips))
		for i, ip := range ips {
			strIPs[i] = ip.String()
		}
		result[d] = strIPs
	}
	return result
}

// Close stops the background refresh goroutine.
func (s *PinnedIPSet) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

// lookupWithTTL resolves the given hostname and returns the IPs and a TTL
// estimate. The Go stdlib net.Resolver.LookupIP does NOT expose the actual
// DNS TTL; v0.4 conservatively returns 5 minutes for all lookups. This is
// acceptable because:
//   - Cloud provider IPs rotate on a multi-hour cadence (5 min refresh is
//     more frequent than necessary).
//   - The pin set retains stale IPs on refresh failure (NFR1 fail-safe).
//
// v0.5 may switch to a custom resolver (e.g., miekg/dns) to get true TTLs.
func lookupWithTTL(r *net.Resolver, domain string) ([]net.IP, time.Duration, error) {
	if ip := net.ParseIP(domain); ip != nil {
		return []net.IP{ip}, 5 * time.Minute, nil
	}
	if domain == "localhost" {
		return []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, 5 * time.Minute, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	if r == nil {
		r = net.DefaultResolver
	}
	ips, err := r.LookupIPAddr(ctx, domain)
	if err != nil {
		return nil, 0, err
	}
	out := make([]net.IP, len(ips))
	for i, ip := range ips {
		out[i] = ip.IP
	}
	// Conservative 5-minute default TTL for stdlib net.Resolver
	return out, 5 * time.Minute, nil
}

// IsDomainAllowed checks if a target hostname matches any allowed domain rule.
// Supports exact matching ("api.openai.com") and wildcard subdomains ("*.googleusercontent.com").
func IsDomainAllowed(target string, allowed []string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if strings.HasPrefix(a, "*.") {
			suffix := a[1:] // ".example.com"
			if strings.HasSuffix(target, suffix) || target == a[2:] {
				return true
			}
		} else if target == a {
			return true
		}
	}
	return false
}

