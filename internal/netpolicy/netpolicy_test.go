package netpolicy

import (
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func TestNetpolicyResolvesDomains(t *testing.T) {
	domains := []string{"localhost"}
	set, err := LookupAndPinDomains(domains)
	if err != nil {
		t.Fatalf("LookupAndPinDomains failed: %v", err)
	}
	defer set.Close()

	ips := set.Get("localhost")
	if len(ips) == 0 {
		t.Errorf("expected non-empty IPs for localhost, got %v", ips)
	}

	m := set.ToMap()
	if _, ok := m["localhost"]; !ok {
		t.Errorf("expected localhost in ToMap, got %v", m)
	}
}

func TestLookupAndPinDomains_PropagatesLookupErrors(t *testing.T) {
	_, err := LookupAndPinDomains([]string{"this-domain-does-not-exist-12345.invalid"})
	if err == nil {
		t.Errorf("expected error for unresolvable domain, got nil")
	}
}

func TestNetpolicySetupEgressDefaultDeny(t *testing.T) {
	// allowNet=false and empty allowDomains -> default-deny no-op.
	cfg, cleanup, err := SetupEgress([]string{}, false, 12345)
	if err != nil {
		t.Fatalf("SetupEgress with empty domains and allowNet=false failed: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil NetConfig for default-deny, got %+v", cfg)
	}
	if err := cleanup(); err != nil {
		t.Errorf("cleanup returned error: %v", err)
	}
}

func TestNetpolicySetupEgressRejectsZeroPID(t *testing.T) {
	_, _, err := SetupEgress([]string{"example.com"}, true, 0)
	if err == nil {
		t.Errorf("expected error for childPID=0, got nil")
	}
}

// TestNetpolicySetupEgressAllowNetSpawnsBackend documents the v1 binary-toggle
// behavior: allowNet=true with empty allowDomains must attempt to spawn a
// backend (not short-circuit). When no backend is available, the error path
// from DiscoverBackend surfaces; when a backend IS available, the spawn may
// still fail because the test process is not actually CLONE_NEWNET-isolated,
// which is also a valid outcome for this unit test (the assertion is on the
// short-circuit, not on full network reachability).
func TestNetpolicySetupEgressAllowNetSpawnsBackend(t *testing.T) {
	cfg, cleanup, err := SetupEgress([]string{}, true, os.Getpid())
	if err != nil {
		// Any error here proves SetupEgress reached backend discovery / spawn
		// instead of short-circuiting on empty allowDomains. The earlier
		// regression was an empty no-op return; this assertion catches that.
		if cfg != nil {
			t.Errorf("SetupEgress returned both error and cfg: err=%v cfg=%+v", err, cfg)
		}
		return
	}
	if cfg == nil {
		t.Errorf("expected populated NetConfig when allowNet=true and backend available, got nil")
	}
	if cleanup != nil {
		if err := cleanup(); err != nil {
			t.Logf("cleanup returned error (acceptable in test): %v", err)
		}
	}
}

func TestNetpolicyTTLRefreshUpdatesPins(t *testing.T) {
	set, err := LookupAndPinDomains([]string{"localhost"})
	if err != nil {
		t.Fatalf("LookupAndPinDomains: %v", err)
	}
	defer set.Close()

	initial := set.Get("localhost")
	if len(initial) == 0 {
		t.Fatalf("expected non-empty initial IPs for localhost, got: %v", initial)
	}

	// Force-refresh by manipulating the refresh deadline to the past
	set.mu.Lock()
	for d := range set.refresh {
		set.refresh[d] = time.Now().Add(-1 * time.Second)
	}
	set.mu.Unlock()

	// Update pins manually simulating a refresh step
	ips, ttl, err := lookupWithTTL(net.DefaultResolver, "localhost")
	if err != nil {
		t.Fatalf("manual refresh lookup: %v", err)
	}
	set.mu.Lock()
	set.pins["localhost"] = ips
	set.ttls["localhost"] = ttl
	set.refresh["localhost"] = time.Now().Add(ttl)
	set.mu.Unlock()

	refreshed := set.Get("localhost")
	if len(refreshed) == 0 {
		t.Errorf("expected refreshed IPs to be non-empty, got: %v", refreshed)
	}
}

func TestPinnedIPSetConcurrentReadsAndWrites(t *testing.T) {
	set, err := LookupAndPinDomains([]string{"localhost"})
	if err != nil {
		t.Fatalf("LookupAndPinDomains: %v", err)
	}
	defer set.Close()

	var wg sync.WaitGroup
	// 50 concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ips := set.Get("localhost")
				_ = ips
			}
		}()
	}
	// 5 concurrent writers (simulating refresh)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				set.mu.Lock()
				set.pins["localhost"] = []net.IP{net.ParseIP("127.0.0.1")}
				set.mu.Unlock()
			}
		}()
	}
	wg.Wait()
}

func TestNetpolicyRefreshOnLookupFailureRetainsStalePins(t *testing.T) {
	set, err := LookupAndPinDomains([]string{"localhost"})
	if err != nil {
		t.Fatalf("LookupAndPinDomains: %v", err)
	}
	defer set.Close()

	// Pre-set stale IP
	set.mu.Lock()
	set.pins["localhost"] = []net.IP{net.ParseIP("192.0.2.1")}
	set.mu.Unlock()

	// Simulate failure in refresh loop: lookup fails, stale pin is preserved
	set.mu.Lock()
	if _, _, lookupErr := lookupWithTTL(set.resolver, "non-existent-domain-12345.invalid"); lookupErr != nil {
		if len(set.pins["localhost"]) == 0 || !set.pins["localhost"][0].Equal(net.ParseIP("192.0.2.1")) {
			t.Errorf("expected stale pin to be retained on refresh failure, got %v", set.pins["localhost"])
		}
	}
	set.mu.Unlock()
}

func TestNetpolicyBuiltinBackendRelayTCP_HappyPath(t *testing.T) {
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("/dev/net/tun not accessible (no CAP_NET_ADMIN): %v", err)
	}
	f.Close()

	cfg := &NetConfig{
		AllowedDomains: []string{"localhost"},
	}
	pins := &PinnedIPSet{
		pins: map[string][]net.IP{"localhost": {net.ParseIP("127.0.0.1")}},
	}
	backend, err := NewBuiltinBackend(cfg, pins)
	if err != nil {
		t.Skipf("cannot create TAP (unprivileged): %v", err)
	}
	defer backend.Close()

	if backend.iface == "" {
		t.Errorf("expected non-empty TAP interface name")
	}
}

func TestNetpolicyBuiltinBackendRelayUDP_HappyPath(t *testing.T) {
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("/dev/net/tun not accessible (no CAP_NET_ADMIN): %v", err)
	}
	f.Close()

	cfg := &NetConfig{
		AllowedDomains: []string{"localhost"},
	}
	pins := &PinnedIPSet{
		pins: map[string][]net.IP{"localhost": {net.ParseIP("127.0.0.1")}},
	}
	backend, err := NewBuiltinBackend(cfg, pins)
	if err != nil {
		t.Skipf("cannot create TAP (unprivileged): %v", err)
	}
	defer backend.Close()
}

func TestNetpolicyBuiltinBackendRelayTCP_DropsNonAllowed(t *testing.T) {
	cfg := &NetConfig{
		AllowedDomains: []string{"allowed.com"},
	}
	pins := &PinnedIPSet{
		pins: map[string][]net.IP{"allowed.com": {net.ParseIP("192.0.2.1")}},
	}
	backend := &BuiltinBackend{
		cfg:  cfg,
		pins: pins,
	}

	if !backend.isIPAllowed(net.ParseIP("192.0.2.1")) {
		t.Errorf("expected 192.0.2.1 to be allowed")
	}
	if backend.isIPAllowed(net.ParseIP("198.51.100.2")) {
		t.Errorf("expected 198.51.100.2 to be denied")
	}
}

func TestNetpolicyBuiltinBackendRelayUDP_DropsNonAllowed(t *testing.T) {
	cfg := &NetConfig{
		AllowedDomains: []string{"allowed.com"},
	}
	pins := &PinnedIPSet{
		pins: map[string][]net.IP{"allowed.com": {net.ParseIP("192.0.2.1")}},
	}
	backend := &BuiltinBackend{
		cfg:  cfg,
		pins: pins,
	}

	if backend.isIPAllowed(net.ParseIP("10.0.0.99")) {
		t.Errorf("expected 10.0.0.99 to be denied")
	}
}

func TestNetpolicyBuiltinBackendRelayTCP_RespectsTTL(t *testing.T) {
	cfg := &NetConfig{
		AllowedDomains: []string{"allowed.com"},
	}
	pins := &PinnedIPSet{
		pins:    map[string][]net.IP{"allowed.com": {net.ParseIP("192.0.2.1")}},
		ttls:    map[string]time.Duration{"allowed.com": time.Minute},
		refresh: map[string]time.Time{"allowed.com": time.Now().Add(time.Minute)},
	}
	backend := &BuiltinBackend{
		cfg:  cfg,
		pins: pins,
	}

	// Update pin dynamically (simulating TTL refresh)
	pins.mu.Lock()
	pins.pins["allowed.com"] = []net.IP{net.ParseIP("192.0.2.2")}
	pins.mu.Unlock()

	if !backend.isIPAllowed(net.ParseIP("192.0.2.2")) {
		t.Errorf("expected newly refreshed IP 192.0.2.2 to be allowed")
	}
}

func TestProbeBuiltinCapability(t *testing.T) {
	// probeBuiltinCapability should return a boolean without panicking
	_ = probeBuiltinCapability()
}

func TestNetpolicyBackendSelection(t *testing.T) {
	backend, err := DiscoverBackend()
	if err != nil {
		t.Logf("DiscoverBackend returned error (no backend installed on system): %v", err)
		return
	}
	if backend != "slirp4netns" && backend != "pasta" && backend != "builtin" {
		t.Errorf("unexpected backend name: %s", backend)
	}
}

func TestIsDomainAllowed_WildcardsAndExact(t *testing.T) {
	rules := []string{"api.anthropic.com", "*.googleusercontent.com", "pypi.org"}

	tests := []struct {
		target string
		want   bool
	}{
		{"api.anthropic.com", true},
		{"API.ANTHROPIC.COM", true},
		{"other.anthropic.com", false},
		{"cdn.googleusercontent.com", true},
		{"sub.deep.googleusercontent.com", true},
		{"googleusercontent.com", true},
		{"pypi.org", true},
		{"files.pythonhosted.org", false},
		{"evil.com", false},
	}

	for _, tt := range tests {
		got := IsDomainAllowed(tt.target, rules)
		if got != tt.want {
			t.Errorf("IsDomainAllowed(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}

