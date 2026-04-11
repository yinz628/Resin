package platform

import (
	"net/netip"
	"regexp"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/testutil"
)

// makeFullyRoutableEntry creates a NodeEntry that passes all 5 filter conditions.
func makeFullyRoutableEntry(hash node.Hash, subIDs ...string) *node.NodeEntry {
	e := node.NewNodeEntry(hash, nil, time.Now(), 16)
	for _, id := range subIDs {
		e.AddSubscriptionID(id)
	}
	// Set all conditions to pass.
	e.LatencyTable.LoadEntry("example.com", node.DomainLatencyStats{
		Ewma:        100 * time.Millisecond,
		LastUpdated: time.Now(),
	})
	ob := testutil.NewNoopOutbound()
	e.Outbound.Store(&ob)
	e.SetEgressIP(testEgressIPForHash(hash))
	return e
}

func testEgressIPForHash(hash node.Hash) netip.Addr {
	return netip.AddrFrom4([4]byte{10, hash[0], hash[1], hash[2]})
}

func alwaysLookup(subID string, hash node.Hash) (string, bool, []string, bool) {
	return "TestSub", true, []string{"us-node", "fast"}, true
}

func usGeoLookup(addr netip.Addr) string { return "us" }

func TestPlatform_EvaluateNode_AllPass(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil) // no filters
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 1 {
		t.Fatalf("expected 1 routable node, got %d", p.View().Size())
	}
}

func TestPlatform_EvaluateNode_CircuitOpen(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil)
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")
	entry.CircuitOpenSince.Store(time.Now().UnixNano()) // circuit open

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 0 {
		t.Fatal("circuit-broken node should not be routable")
	}
}

func TestPlatform_EvaluateNode_NoLatency(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil)
	h := makeHash(`{"type":"ss"}`)
	// Create entry without latency table (maxLatencyTableEntries=0).
	entry := node.NewNodeEntry(h, nil, time.Now(), 0)
	entry.AddSubscriptionID("sub1")
	ob := testutil.NewNoopOutbound()
	entry.Outbound.Store(&ob)
	entry.SetEgressIP(netip.MustParseAddr("1.2.3.4"))

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 0 {
		t.Fatal("node without latency should not be routable")
	}
}

func TestPlatform_EvaluateNode_NoOutbound(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil)
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")
	entry.Outbound.Store(nil) // no outbound

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 0 {
		t.Fatal("node without outbound should not be routable")
	}
}

func TestPlatform_EvaluateNode_NoEgressIP(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil) // no region filters
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")
	entry.SetEgressIP(netip.Addr{}) // egress unknown

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 0 {
		t.Fatal("node without egress IP should not be routable")
	}
}

func TestPlatform_EvaluateNode_RegexFilter(t *testing.T) {
	regexes := []*regexp.Regexp{regexp.MustCompile("us")}
	p := NewPlatform("p1", "Test", regexes, nil)
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")

	// Lookup returns "TestSub/us-node" which matches "us".
	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 1 {
		t.Fatal("node matching regex should be routable")
	}

	// Now with a "jp" filter — should NOT match.
	p2 := NewPlatform("p2", "Test", []*regexp.Regexp{regexp.MustCompile("^jp")}, nil)
	p2.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p2.View().Size() != 0 {
		t.Fatal("node not matching regex should not be routable")
	}
}

func TestPlatform_EvaluateNode_RegionFilter(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, []string{"us"})
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 1 {
		t.Fatal("node in allowed region should be routable")
	}

	// Region filter "jp" — node has US egress, should fail.
	p2 := NewPlatform("p2", "Test", nil, []string{"jp"})
	p2.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p2.View().Size() != 0 {
		t.Fatal("node not in allowed region should not be routable")
	}
}

func TestPlatform_EvaluateNode_RegionFilter_NoEgressIP(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, []string{"us"})
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")
	// Don't set egress IP — clear it.
	entry.SetEgressIP(netip.Addr{})

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 0 {
		t.Fatal("node without egress IP should not be routable")
	}
}

func TestPlatform_EvaluateNode_ServiceFilter(t *testing.T) {
	hOpenAI := makeHash(`{"type":"ss","name":"openai"}`)
	hUnsupported := makeHash(`{"type":"ss","name":"unsupported"}`)

	openAIEntry := makeFullyRoutableEntry(hOpenAI, "sub1")
	openAIEntry.SetServiceCapabilities(true, false)

	unsupportedEntry := makeFullyRoutableEntry(hUnsupported, "sub1")
	unsupportedEntry.SetServiceCapabilities(false, false)

	p := NewPlatform("p1", "Test", nil, nil)
	p.ServiceFilters = []string{"openai"}
	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(hOpenAI, openAIEntry)
		fn(hUnsupported, unsupportedEntry)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 1 || !p.View().Contains(hOpenAI) {
		t.Fatalf("expected only openai node to match, size=%d", p.View().Size())
	}
}

func TestPlatform_EvaluateNode_RegionFilter_PrefersStoredRegion(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, []string{"jp"})
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")
	entry.SetEgressRegion("jp")

	geoCalled := false
	geoLookup := func(netip.Addr) string {
		geoCalled = true
		return "us"
	}

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, geoLookup)

	if p.View().Size() != 1 {
		t.Fatal("stored region should be used before GeoIP fallback")
	}
	if geoCalled {
		t.Fatal("GeoIP lookup should be skipped when stored region exists")
	}
}

func TestPlatform_EvaluateNode_RegionFilter_ExcludeOnlyUnknownRegion(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, []string{"!hk"})
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")

	geoLookup := func(netip.Addr) string { return "" }
	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h, entry)
	}, alwaysLookup, geoLookup)

	if p.View().Size() != 0 {
		t.Fatal("node with unknown region should not be routable when region filters are configured")
	}
}

func TestMatchRegionFilter(t *testing.T) {
	tests := []struct {
		name    string
		filters []string
		region  string
		want    bool
	}{
		{
			name:    "include only match",
			filters: []string{"hk", "us"},
			region:  "hk",
			want:    true,
		},
		{
			name:    "include only miss",
			filters: []string{"hk", "us"},
			region:  "jp",
			want:    false,
		},
		{
			name:    "exclude only",
			filters: []string{"!hk"},
			region:  "us",
			want:    true,
		},
		{
			name:    "exclude only blocked",
			filters: []string{"!hk"},
			region:  "hk",
			want:    false,
		},
		{
			name:    "exclude only unknown region",
			filters: []string{"!hk"},
			region:  "",
			want:    false,
		},
		{
			name:    "mixed include and exclude allows expected",
			filters: []string{"hk", "!us"},
			region:  "hk",
			want:    true,
		},
		{
			name:    "mixed include and exclude blocks excluded",
			filters: []string{"hk", "!us"},
			region:  "us",
			want:    false,
		},
		{
			name:    "mixed include and same exclude blocks",
			filters: []string{"hk", "!hk"},
			region:  "hk",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchRegionFilter(tt.region, tt.filters); got != tt.want {
				t.Fatalf("MatchRegionFilter(%q, %v) = %v, want %v", tt.region, tt.filters, got, tt.want)
			}
		})
	}
}

func TestPlatform_NotifyDirty_AddRemove(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil)
	h := makeHash(`{"type":"ss"}`)
	entry := makeFullyRoutableEntry(h, "sub1")

	entryStore := map[node.Hash]*node.NodeEntry{h: entry}
	getEntry := func(hash node.Hash) (*node.NodeEntry, bool) {
		e, ok := entryStore[hash]
		return e, ok
	}

	// Initially empty — add via NotifyDirty.
	p.NotifyDirty(h, getEntry, alwaysLookup, usGeoLookup)
	if p.View().Size() != 1 {
		t.Fatal("NotifyDirty should add passing node")
	}

	// Circuit-break → NotifyDirty removes.
	entry.CircuitOpenSince.Store(time.Now().UnixNano())
	p.NotifyDirty(h, getEntry, alwaysLookup, usGeoLookup)
	if p.View().Size() != 0 {
		t.Fatal("NotifyDirty should remove circuit-broken node")
	}

	// Recover → NotifyDirty re-adds.
	entry.CircuitOpenSince.Store(0)
	p.NotifyDirty(h, getEntry, alwaysLookup, usGeoLookup)
	if p.View().Size() != 1 {
		t.Fatal("NotifyDirty should re-add recovered node")
	}

	// Delete from pool → NotifyDirty removes.
	delete(entryStore, h)
	p.NotifyDirty(h, getEntry, alwaysLookup, usGeoLookup)
	if p.View().Size() != 0 {
		t.Fatal("NotifyDirty should remove deleted node")
	}
}

func TestPlatform_FullRebuild_ClearsOld(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil)
	h1 := makeHash(`{"type":"ss","n":1}`)
	h2 := makeHash(`{"type":"ss","n":2}`)
	e1 := makeFullyRoutableEntry(h1, "sub1")
	e2 := makeFullyRoutableEntry(h2, "sub1")

	// First rebuild with 2 nodes.
	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h1, e1)
		fn(h2, e2)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 2 {
		t.Fatalf("expected 2, got %d", p.View().Size())
	}

	// Second rebuild with only 1 node — old entries cleared.
	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h1, e1)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 1 {
		t.Fatalf("expected 1 after rebuild, got %d", p.View().Size())
	}
	if p.View().Contains(h2) {
		t.Fatal("h2 should have been removed by rebuild")
	}
}

func TestPlatform_FullRebuild_DeduplicatesByEgressIP(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil)
	p.DeduplicateEgressIP = true

	h1 := makeHash(`{"type":"ss","n":1}`)
	h2 := makeHash(`{"type":"ss","n":2}`)
	h3 := makeHash(`{"type":"ss","n":3}`)
	e1 := makeFullyRoutableEntry(h1, "sub1")
	e2 := makeFullyRoutableEntry(h2, "sub1")
	e3 := makeFullyRoutableEntry(h3, "sub1")

	sameIP := netip.MustParseAddr("1.2.3.4")
	e1.SetEgressIP(sameIP)
	e2.SetEgressIP(sameIP)
	e3.SetEgressIP(netip.MustParseAddr("5.6.7.8"))

	p.FullRebuild(func(fn func(node.Hash, *node.NodeEntry) bool) {
		fn(h1, e1)
		fn(h2, e2)
		fn(h3, e3)
	}, alwaysLookup, usGeoLookup)

	if p.View().Size() != 2 {
		t.Fatalf("expected 2 deduplicated routable nodes, got %d", p.View().Size())
	}
	winner := h1
	loser := h2
	if !isHashLess(h1, h2) {
		winner = h2
		loser = h1
	}
	if !p.View().Contains(winner) {
		t.Fatal("expected deterministic winner hash to remain in view")
	}
	if p.View().Contains(loser) {
		t.Fatal("expected duplicate egress-ip node to be excluded from view")
	}
	if !p.View().Contains(h3) {
		t.Fatal("expected distinct egress-ip node to remain in view")
	}
}

func TestPlatform_NotifyDirtyWithPoolRange_PromotesFallbackForSameEgressIP(t *testing.T) {
	p := NewPlatform("p1", "Test", nil, nil)
	p.DeduplicateEgressIP = true

	h1 := makeHash(`{"type":"ss","n":1}`)
	h2 := makeHash(`{"type":"ss","n":2}`)
	e1 := makeFullyRoutableEntry(h1, "sub1")
	e2 := makeFullyRoutableEntry(h2, "sub1")
	sharedIP := netip.MustParseAddr("9.9.9.9")
	e1.SetEgressIP(sharedIP)
	e2.SetEgressIP(sharedIP)

	store := map[node.Hash]*node.NodeEntry{
		h1: e1,
		h2: e2,
	}
	getEntry := func(hash node.Hash) (*node.NodeEntry, bool) {
		entry, ok := store[hash]
		return entry, ok
	}
	poolRange := func(fn func(node.Hash, *node.NodeEntry) bool) {
		for h, e := range store {
			if !fn(h, e) {
				return
			}
		}
	}

	p.FullRebuild(poolRange, alwaysLookup, usGeoLookup)
	if p.View().Size() != 1 {
		t.Fatalf("expected 1 node after initial dedupe rebuild, got %d", p.View().Size())
	}

	owner := h1
	fallback := h2
	if !isHashLess(h1, h2) {
		owner = h2
		fallback = h1
	}
	if !p.View().Contains(owner) {
		t.Fatal("expected initial owner to be selected")
	}

	delete(store, owner)
	p.NotifyDirtyWithPoolRange(owner, getEntry, poolRange, alwaysLookup, usGeoLookup)

	if p.View().Size() != 1 {
		t.Fatalf("expected 1 node after owner removal fallback, got %d", p.View().Size())
	}
	if !p.View().Contains(fallback) {
		t.Fatal("expected fallback node with same egress IP to be promoted")
	}
}
