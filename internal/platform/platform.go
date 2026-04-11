package platform

import (
	"bytes"
	"net/netip"
	"regexp"
	"sync"

	"github.com/Resinat/Resin/internal/node"
)

// DefaultPlatformID is the well-known UUID of the built-in Default platform.
const DefaultPlatformID = "00000000-0000-0000-0000-000000000000"

// DefaultPlatformName is the built-in platform name.
const DefaultPlatformName = "Default"

// GeoLookupFunc resolves an IP address to a lowercase ISO country code.
type GeoLookupFunc func(netip.Addr) string

// PoolRangeFunc iterates all nodes in the global pool.
type PoolRangeFunc func(fn func(node.Hash, *node.NodeEntry) bool)

// GetEntryFunc retrieves a node entry from the global pool by hash.
type GetEntryFunc func(node.Hash) (*node.NodeEntry, bool)

// Platform represents a routing platform with its filtered routable view.
type Platform struct {
	ID   string
	Name string

	// Filter configuration.
	RegexFilters   []*regexp.Regexp
	RegionFilters  []string // lowercase ISO codes, supports negation "!xx"
	ServiceFilters []string // openai/anthropic/unsupported

	// Other config fields.
	StickyTTLNs                      int64
	ReverseProxyMissAction           string
	ReverseProxyEmptyAccountBehavior string
	ReverseProxyFixedAccountHeader   string
	ReverseProxyFixedAccountHeaders  []string
	AllocationPolicy                 AllocationPolicy
	// DeduplicateEgressIP keeps at most one routable node per egress IP.
	// This is enabled for runtime platforms created from persisted models.
	DeduplicateEgressIP bool

	// Routable view and its lock.
	// viewMu serializes both FullRebuild and NotifyDirty.
	view        *RoutableView
	viewMu      sync.Mutex
	ownerByIP   map[netip.Addr]node.Hash
	ownerByHash map[node.Hash]netip.Addr
}

// NewPlatform creates a Platform with an empty routable view.
func NewPlatform(id, name string, regexFilters []*regexp.Regexp, regionFilters []string) *Platform {
	return &Platform{
		ID:            id,
		Name:          name,
		RegexFilters:  regexFilters,
		RegionFilters: regionFilters,
		view:          NewRoutableView(),
		ownerByIP:     make(map[netip.Addr]node.Hash),
		ownerByHash:   make(map[node.Hash]netip.Addr),
	}
}

// View returns the platform's routable view as a read-only interface.
// External callers cannot Add/Remove/Clear; only FullRebuild and NotifyDirty can mutate.
func (p *Platform) View() ReadOnlyView {
	return p.view
}

// FullRebuild clears the routable view and re-evaluates all nodes from the pool.
// Acquires viewMu; any concurrent NotifyDirty calls block until rebuild completes.
func (p *Platform) FullRebuild(
	poolRange PoolRangeFunc,
	subLookup node.SubLookupFunc,
	geoLookup GeoLookupFunc,
) {
	p.viewMu.Lock()
	defer p.viewMu.Unlock()

	p.view.Clear()
	p.resetOwnersLocked()

	if !p.DeduplicateEgressIP {
		poolRange(func(h node.Hash, entry *node.NodeEntry) bool {
			if p.evaluateNode(entry, subLookup, geoLookup) {
				p.view.Add(h)
			}
			return true
		})
		return
	}

	nextOwners := make(map[netip.Addr]node.Hash)
	poolRange(func(h node.Hash, entry *node.NodeEntry) bool {
		if !p.evaluateNode(entry, subLookup, geoLookup) {
			return true
		}
		ip := entry.GetEgressIP()
		if current, exists := nextOwners[ip]; !exists || isHashLess(h, current) {
			nextOwners[ip] = h
		}
		return true
	})

	for ip, h := range nextOwners {
		p.ownerByIP[ip] = h
		p.ownerByHash[h] = ip
		p.view.Add(h)
	}
}

// NotifyDirty re-evaluates a single node and adds/removes it from the view.
// Acquires viewMu; serialized with FullRebuild.
func (p *Platform) NotifyDirty(
	h node.Hash,
	getEntry GetEntryFunc,
	subLookup node.SubLookupFunc,
	geoLookup GeoLookupFunc,
) {
	p.NotifyDirtyWithPoolRange(h, getEntry, nil, subLookup, geoLookup)
}

// NotifyDirtyWithPoolRange re-evaluates a single node and updates the view.
// When egress dedupe is enabled and poolRange is provided, removing the current
// owner of an egress IP will promote another eligible node with the same IP.
func (p *Platform) NotifyDirtyWithPoolRange(
	h node.Hash,
	getEntry GetEntryFunc,
	poolRange PoolRangeFunc,
	subLookup node.SubLookupFunc,
	geoLookup GeoLookupFunc,
) {
	p.viewMu.Lock()
	defer p.viewMu.Unlock()

	if !p.DeduplicateEgressIP {
		entry, ok := getEntry(h)
		if !ok {
			p.view.Remove(h)
			return
		}
		if p.evaluateNode(entry, subLookup, geoLookup) {
			p.view.Add(h)
		} else {
			p.view.Remove(h)
		}
		return
	}

	previousIP, wasOwner := p.ownerByHash[h]
	entry, ok := getEntry(h)
	if !ok {
		p.view.Remove(h)
		if wasOwner {
			delete(p.ownerByHash, h)
			delete(p.ownerByIP, previousIP)
			p.recomputeOwnerForIPLocked(previousIP, poolRange, subLookup, geoLookup)
		}
		return
	}

	if !p.evaluateNode(entry, subLookup, geoLookup) {
		p.view.Remove(h)
		if wasOwner {
			delete(p.ownerByHash, h)
			delete(p.ownerByIP, previousIP)
			p.recomputeOwnerForIPLocked(previousIP, poolRange, subLookup, geoLookup)
		}
		return
	}

	currentIP := entry.GetEgressIP()
	if wasOwner && previousIP != currentIP {
		delete(p.ownerByHash, h)
		delete(p.ownerByIP, previousIP)
		p.view.Remove(h)
		p.recomputeOwnerForIPLocked(previousIP, poolRange, subLookup, geoLookup)
	}

	if ownerHash, exists := p.ownerByIP[currentIP]; !exists {
		p.ownerByIP[currentIP] = h
		p.ownerByHash[h] = currentIP
		p.view.Add(h)
		return
	} else if ownerHash == h {
		p.ownerByHash[h] = currentIP
		p.view.Add(h)
		return
	} else if isHashLess(h, ownerHash) {
		p.view.Remove(ownerHash)
		delete(p.ownerByHash, ownerHash)
		p.ownerByIP[currentIP] = h
		p.ownerByHash[h] = currentIP
		p.view.Add(h)
		return
	}

	p.view.Remove(h)
	delete(p.ownerByHash, h)
}

// evaluateNode checks all filter conditions for platform routability.
func (p *Platform) evaluateNode(
	entry *node.NodeEntry,
	subLookup node.SubLookupFunc,
	geoLookup GeoLookupFunc,
) bool {
	// 0. Disabled nodes are never routable.
	if entry.IsDisabledBySubscriptions(subLookup) {
		return false
	}

	// 1. Healthy for routing (outbound ready + circuit not open).
	if !entry.IsHealthy() {
		return false
	}

	// 2. Tag regex match.
	if !entry.MatchRegexs(p.RegexFilters, subLookup) {
		return false
	}

	// 3. Egress IP must be known.
	egressIP := entry.GetEgressIP()
	if !egressIP.IsValid() {
		return false
	}

	// 4. Region filter (when configured).
	if len(p.RegionFilters) > 0 {
		region := entry.GetRegion(geoLookup)
		if !MatchRegionFilter(region, p.RegionFilters) {
			return false
		}
	}

	// 5. Service capability filter (when configured).
	if len(p.ServiceFilters) > 0 {
		if !MatchServiceFilters(entry.SupportsOpenAI(), entry.SupportsAnthropic(), p.ServiceFilters) {
			return false
		}
	}

	// 6. Has at least one latency record.
	if !entry.HasLatency() {
		return false
	}

	return true
}

// MatchRegionFilter applies include/exclude region filters.
// Positive entries (xx) build an include set; negative entries (!xx) build an exclude set.
// Unknown regions never match when region filters are configured.
// Final result is: region known AND (include empty OR region in include) AND (region not in exclude).
func MatchRegionFilter(region string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	if region == "" {
		return false
	}

	included := false
	hasInclude := false

	for _, filter := range filters {
		if len(filter) > 0 && filter[0] == '!' {
			if region == filter[1:] {
				return false
			}
			continue
		}
		hasInclude = true
		if region == filter {
			included = true
		}
	}

	if hasInclude && !included {
		return false
	}
	return true
}

func (p *Platform) resetOwnersLocked() {
	clear(p.ownerByIP)
	clear(p.ownerByHash)
}

func (p *Platform) recomputeOwnerForIPLocked(
	targetIP netip.Addr,
	poolRange PoolRangeFunc,
	subLookup node.SubLookupFunc,
	geoLookup GeoLookupFunc,
) {
	if poolRange == nil {
		return
	}

	var (
		best  node.Hash
		found bool
	)
	poolRange(func(h node.Hash, entry *node.NodeEntry) bool {
		if entry == nil {
			return true
		}
		if entry.GetEgressIP() != targetIP {
			return true
		}
		if !p.evaluateNode(entry, subLookup, geoLookup) {
			return true
		}
		if !found || isHashLess(h, best) {
			best = h
			found = true
		}
		return true
	})
	if !found {
		return
	}
	p.ownerByIP[targetIP] = best
	p.ownerByHash[best] = targetIP
	p.view.Add(best)
}

func isHashLess(a, b node.Hash) bool {
	return bytes.Compare(a[:], b[:]) < 0
}
