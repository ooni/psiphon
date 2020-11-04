/*
 * Copyright (c) 2016, Psiphon Inc.
 * All rights reserved.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package server

import (
	"encoding/json"
	"net"
	"time"

	"github.com/ooni/psiphon/oopsi/github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common"
	"github.com/ooni/psiphon/oopsi/github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/errors"
)

const (
	DEFAULT_IDLE_TCP_PORT_FORWARD_TIMEOUT_MILLISECONDS        = 30000
	DEFAULT_IDLE_UDP_PORT_FORWARD_TIMEOUT_MILLISECONDS        = 30000
	DEFAULT_DIAL_TCP_PORT_FORWARD_TIMEOUT_MILLISECONDS        = 10000
	DEFAULT_MAX_TCP_DIALING_PORT_FORWARD_COUNT                = 64
	DEFAULT_MAX_TCP_PORT_FORWARD_COUNT                        = 512
	DEFAULT_MAX_UDP_PORT_FORWARD_COUNT                        = 32
	DEFAULT_MEEK_RATE_LIMITER_GARBAGE_COLLECTOR_TRIGGER_COUNT = 5000
	DEFAULT_MEEK_RATE_LIMITER_REAP_HISTORY_FREQUENCY_SECONDS  = 600
)

// TrafficRulesSet represents the various traffic rules to
// apply to Psiphon client tunnels. The Reload function supports
// hot reloading of rules data while the server is running.
//
// For a given client, the traffic rules are determined by starting
// with DefaultRules, then finding the first (if any)
// FilteredTrafficRules match and overriding the defaults with fields
// set in the selected FilteredTrafficRules.
type TrafficRulesSet struct {
	common.ReloadableFile

	// DefaultRules are the base values to use as defaults for all
	// clients.
	DefaultRules TrafficRules

	// FilteredTrafficRules is an ordered list of filter/rules pairs.
	// For each client, the first matching Filter in FilteredTrafficRules
	// determines the additional Rules that are selected and applied
	// on top of DefaultRules.
	FilteredRules []struct {
		Filter TrafficRulesFilter
		Rules  TrafficRules
	}

	// MeekRateLimiterHistorySize enables the late-stage meek rate limiter and
	// sets its history size. The late-stage meek rate limiter acts on client
	// IPs relayed in MeekProxyForwardedForHeaders, and so it must wait for
	// the HTTP headers to be read. This rate limiter immediately terminates
	// any client endpoint request or any request to create a new session, but
	// not any meek request for an existing session, if the
	// MeekRateLimiterHistorySize requests occur in
	// MeekRateLimiterThresholdSeconds. The scope of rate limiting may be
	// limited using LimitMeekRateLimiterRegions/ISPs/Cities.
	//
	// Hot reloading a new history size will result in existing history being
	// truncated.
	MeekRateLimiterHistorySize int

	// MeekRateLimiterThresholdSeconds is part of the meek rate limiter
	// specification and must be set when MeekRateLimiterHistorySize is set.
	MeekRateLimiterThresholdSeconds int

	// MeekRateLimiterRegions, if set, limits application of the meek
	// late-stage rate limiter to clients in the specified list of GeoIP
	// countries. When omitted or empty, meek rate limiting, if configured,
	// is applied to all client countries.
	MeekRateLimiterRegions []string

	// MeekRateLimiterISPs, if set, limits application of the meek
	// late-stage rate limiter to clients in the specified list of GeoIP
	// ISPs. When omitted or empty, meek rate limiting, if configured,
	// is applied to all client ISPs.
	MeekRateLimiterISPs []string

	// MeekRateLimiterCities, if set, limits application of the meek
	// late-stage rate limiter to clients in the specified list of GeoIP
	// cities. When omitted or empty, meek rate limiting, if configured,
	// is applied to all client cities.
	MeekRateLimiterCities []string

	// MeekRateLimiterGarbageCollectionTriggerCount specifies the number of
	// rate limit events after which garbage collection is manually triggered
	// in order to reclaim memory used by rate limited and other rejected
	// requests.
	// A default of 5000 is used when
	// MeekRateLimiterGarbageCollectionTriggerCount is 0.
	MeekRateLimiterGarbageCollectionTriggerCount int

	// MeekRateLimiterReapHistoryFrequencySeconds specifies a schedule for
	// reaping old records from the rate limit history.
	// A default of 600 is used when
	// MeekRateLimiterReapHistoryFrequencySeconds is 0.
	MeekRateLimiterReapHistoryFrequencySeconds int
}

// TrafficRulesFilter defines a filter to match against client attributes.
type TrafficRulesFilter struct {

	// TunnelProtocols is a list of client tunnel protocols that must be
	// in use to match this filter. When omitted or empty, any protocol
	// matches.
	TunnelProtocols []string

	// Regions is a list of countries that the client must geolocate to in
	// order to match this filter. When omitted or empty, any client country
	// matches.
	Regions []string

	// ISPs is a list of ISPs that the client must geolocate to in order to
	// match this filter. When omitted or empty, any client ISP matches.
	ISPs []string

	// Cities is a list of cities that the client must geolocate to in order to
	// match this filter. When omitted or empty, any client city matches.
	Cities []string

	// APIProtocol specifies whether the client must use the SSH
	// API protocol (when "ssh") or the web API protocol (when "web").
	// When omitted or blank, any API protocol matches.
	APIProtocol string

	// HandshakeParameters specifies handshake API parameter names and
	// a list of values, one of which must be specified to match this
	// filter. Only scalar string API parameters may be filtered.
	// Values may be patterns containing the '*' wildcard.
	HandshakeParameters map[string][]string

	// AuthorizedAccessTypes specifies a list of access types, at least
	// one of which the client must have presented an active authorization
	// for and which must not be revoked.
	// AuthorizedAccessTypes is ignored when AuthorizationsRevoked is true.
	AuthorizedAccessTypes []string

	// ActiveAuthorizationIDs specifies a list of authorization IDs, at least
	// one of which the client must have presented an active authorization
	// for and which must not be revoked.
	// ActiveAuthorizationIDs is ignored when AuthorizationsRevoked is true.
	ActiveAuthorizationIDs []string

	// AuthorizationsRevoked indicates whether the client's authorizations
	// must have been revoked. When true, authorizations must have been
	// revoked. When omitted or false, this field is ignored.
	AuthorizationsRevoked bool

	regionLookup                map[string]bool
	ispLookup                   map[string]bool
	cityLookup                  map[string]bool
	activeAuthorizationIDLookup map[string]bool
}

// TrafficRules specify the limits placed on client traffic.
type TrafficRules struct {

	// RateLimits specifies data transfer rate limits for the
	// client traffic.
	RateLimits RateLimits

	// DialTCPPortForwardTimeoutMilliseconds is the timeout period
	// for dialing TCP port forwards. A value of 0 specifies no timeout.
	// When omitted in DefaultRules,
	// DEFAULT_TCP_PORT_FORWARD_DIAL_TIMEOUT_MILLISECONDS is used.
	DialTCPPortForwardTimeoutMilliseconds *int

	// IdleTCPPortForwardTimeoutMilliseconds is the timeout period
	// after which idle (no bytes flowing in either direction)
	// client TCP port forwards are preemptively closed.
	// A value of 0 specifies no idle timeout. When omitted in
	// DefaultRules, DEFAULT_IDLE_TCP_PORT_FORWARD_TIMEOUT_MILLISECONDS
	// is used.
	IdleTCPPortForwardTimeoutMilliseconds *int

	// IdleUDPPortForwardTimeoutMilliseconds is the timeout period
	// after which idle (no bytes flowing in either direction)
	// client UDP port forwards are preemptively closed.
	// A value of 0 specifies no idle timeout. When omitted in
	// DefaultRules, DEFAULT_IDLE_UDP_PORT_FORWARD_TIMEOUT_MILLISECONDS
	// is used.
	IdleUDPPortForwardTimeoutMilliseconds *int

	// MaxTCPDialingPortForwardCount is the maximum number of dialing
	// TCP port forwards each client may have open concurrently. When
	// persistently at the limit, new TCP port forwards are rejected.
	// A value of 0 specifies no maximum. When omitted in
	// DefaultRules, DEFAULT_MAX_TCP_DIALING_PORT_FORWARD_COUNT is used.
	MaxTCPDialingPortForwardCount *int

	// MaxTCPPortForwardCount is the maximum number of established TCP
	// port forwards each client may have open concurrently. If at the
	// limit when a new TCP port forward is established, the LRU
	// established TCP port forward is closed.
	// A value of 0 specifies no maximum. When omitted in
	// DefaultRules, DEFAULT_MAX_TCP_PORT_FORWARD_COUNT is used.
	MaxTCPPortForwardCount *int

	// MaxUDPPortForwardCount is the maximum number of UDP port
	// forwards each client may have open concurrently. If at the
	// limit when a new UDP port forward is created, the LRU
	// UDP port forward is closed.
	// A value of 0 specifies no maximum. When omitted in
	// DefaultRules, DEFAULT_MAX_UDP_PORT_FORWARD_COUNT is used.
	MaxUDPPortForwardCount *int

	// AllowTCPPorts specifies a list of TCP ports that are permitted for port
	// forwarding. When set, only ports in the list are accessible to clients.
	AllowTCPPorts []int

	// AllowUDPPorts specifies a list of UDP ports that are permitted for port
	// forwarding. When set, only ports in the list are accessible to clients.
	AllowUDPPorts []int

	// DisallowTCPPorts specifies a list of TCP ports that are not permitted for
	// port forwarding. DisallowTCPPorts takes priority over AllowTCPPorts and
	// AllowSubnets.
	DisallowTCPPorts []int

	// DisallowUDPPorts specifies a list of UDP ports that are not permitted for
	// port forwarding. DisallowUDPPorts takes priority over AllowUDPPorts and
	// AllowSubnets.
	DisallowUDPPorts []int

	// AllowSubnets specifies a list of IP address subnets for which all TCP and
	// UDP ports are allowed. This list is consulted if a port is disallowed by
	// the AllowTCPPorts or AllowUDPPorts configuration. Each entry is a IP
	// subnet in CIDR notation.
	//
	// Limitation: currently, AllowSubnets only matches port forwards where the
	// client sends an IP address. Domain names are not resolved before checking
	// AllowSubnets.
	AllowSubnets []string

	allowTCPPortsLookup    map[int]bool
	allowUDPPortsLookup    map[int]bool
	disallowTCPPortsLookup map[int]bool
	disallowUDPPortsLookup map[int]bool
}

// RateLimits is a clone of common.RateLimits with pointers
// to fields to enable distinguishing between zero values and
// omitted values in JSON serialized traffic rules.
// See common.RateLimits for field descriptions.
type RateLimits struct {
	ReadUnthrottledBytes  *int64
	ReadBytesPerSecond    *int64
	WriteUnthrottledBytes *int64
	WriteBytesPerSecond   *int64
	CloseAfterExhausted   *bool

	// UnthrottleFirstTunnelOnly specifies whether any
	// ReadUnthrottledBytes/WriteUnthrottledBytes apply
	// only to the first tunnel in a session.
	UnthrottleFirstTunnelOnly *bool
}

// CommonRateLimits converts a RateLimits to a common.RateLimits.
func (rateLimits *RateLimits) CommonRateLimits() common.RateLimits {
	return common.RateLimits{
		ReadUnthrottledBytes:  *rateLimits.ReadUnthrottledBytes,
		ReadBytesPerSecond:    *rateLimits.ReadBytesPerSecond,
		WriteUnthrottledBytes: *rateLimits.WriteUnthrottledBytes,
		WriteBytesPerSecond:   *rateLimits.WriteBytesPerSecond,
		CloseAfterExhausted:   *rateLimits.CloseAfterExhausted,
	}
}

// NewTrafficRulesSet initializes a TrafficRulesSet with
// the rules data in the specified config file.
func NewTrafficRulesSet(filename string) (*TrafficRulesSet, error) {

	set := &TrafficRulesSet{}

	set.ReloadableFile = common.NewReloadableFile(
		filename,
		true,
		func(fileContent []byte, _ time.Time) error {
			var newSet TrafficRulesSet
			err := json.Unmarshal(fileContent, &newSet)
			if err != nil {
				return errors.Trace(err)
			}
			err = newSet.Validate()
			if err != nil {
				return errors.Trace(err)
			}

			// Modify actual traffic rules only after validation
			set.MeekRateLimiterHistorySize = newSet.MeekRateLimiterHistorySize
			set.MeekRateLimiterThresholdSeconds = newSet.MeekRateLimiterThresholdSeconds
			set.MeekRateLimiterRegions = newSet.MeekRateLimiterRegions
			set.MeekRateLimiterISPs = newSet.MeekRateLimiterISPs
			set.MeekRateLimiterCities = newSet.MeekRateLimiterCities
			set.MeekRateLimiterGarbageCollectionTriggerCount = newSet.MeekRateLimiterGarbageCollectionTriggerCount
			set.MeekRateLimiterReapHistoryFrequencySeconds = newSet.MeekRateLimiterReapHistoryFrequencySeconds
			set.DefaultRules = newSet.DefaultRules
			set.FilteredRules = newSet.FilteredRules

			set.initLookups()

			return nil
		})

	_, err := set.Reload()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return set, nil
}

// Validate checks for correct input formats in a TrafficRulesSet.
func (set *TrafficRulesSet) Validate() error {

	if set.MeekRateLimiterHistorySize < 0 ||
		set.MeekRateLimiterThresholdSeconds < 0 ||
		set.MeekRateLimiterGarbageCollectionTriggerCount < 0 ||
		set.MeekRateLimiterReapHistoryFrequencySeconds < 0 {
		return errors.TraceNew("MeekRateLimiter values must be >= 0")
	}

	if set.MeekRateLimiterHistorySize > 0 {
		if set.MeekRateLimiterThresholdSeconds <= 0 {
			return errors.TraceNew("MeekRateLimiterThresholdSeconds must be > 0")
		}
	}

	validateTrafficRules := func(rules *TrafficRules) error {

		if (rules.RateLimits.ReadUnthrottledBytes != nil && *rules.RateLimits.ReadUnthrottledBytes < 0) ||
			(rules.RateLimits.ReadBytesPerSecond != nil && *rules.RateLimits.ReadBytesPerSecond < 0) ||
			(rules.RateLimits.WriteUnthrottledBytes != nil && *rules.RateLimits.WriteUnthrottledBytes < 0) ||
			(rules.RateLimits.WriteBytesPerSecond != nil && *rules.RateLimits.WriteBytesPerSecond < 0) ||
			(rules.DialTCPPortForwardTimeoutMilliseconds != nil && *rules.DialTCPPortForwardTimeoutMilliseconds < 0) ||
			(rules.IdleTCPPortForwardTimeoutMilliseconds != nil && *rules.IdleTCPPortForwardTimeoutMilliseconds < 0) ||
			(rules.IdleUDPPortForwardTimeoutMilliseconds != nil && *rules.IdleUDPPortForwardTimeoutMilliseconds < 0) ||
			(rules.MaxTCPDialingPortForwardCount != nil && *rules.MaxTCPDialingPortForwardCount < 0) ||
			(rules.MaxTCPPortForwardCount != nil && *rules.MaxTCPPortForwardCount < 0) ||
			(rules.MaxUDPPortForwardCount != nil && *rules.MaxUDPPortForwardCount < 0) {
			return errors.TraceNew("TrafficRules values must be >= 0")
		}

		for _, subnet := range rules.AllowSubnets {
			_, _, err := net.ParseCIDR(subnet)
			if err != nil {
				return errors.Tracef("invalid subnet: %s %s", subnet, err)
			}
		}

		return nil
	}

	err := validateTrafficRules(&set.DefaultRules)
	if err != nil {
		return errors.Trace(err)
	}

	for _, filteredRule := range set.FilteredRules {

		for paramName := range filteredRule.Filter.HandshakeParameters {
			validParamName := false
			for _, paramSpec := range handshakeRequestParams {
				if paramSpec.name == paramName {
					validParamName = true
					break
				}
			}
			if !validParamName {
				return errors.Tracef("invalid parameter name: %s", paramName)
			}
		}

		err := validateTrafficRules(&filteredRule.Rules)
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

const stringLookupThreshold = 5
const intLookupThreshold = 10

// initLookups creates map lookups for filters where the number of string/int
// values to compare against exceeds a threshold where benchmarks show maps
// are faster than looping through a string/int slice.
func (set *TrafficRulesSet) initLookups() {

	initTrafficRulesLookups := func(rules *TrafficRules) {

		if len(rules.AllowTCPPorts) >= intLookupThreshold {
			rules.allowTCPPortsLookup = make(map[int]bool)
			for _, port := range rules.AllowTCPPorts {
				rules.allowTCPPortsLookup[port] = true
			}
		}

		if len(rules.AllowUDPPorts) >= intLookupThreshold {
			rules.allowUDPPortsLookup = make(map[int]bool)
			for _, port := range rules.AllowUDPPorts {
				rules.allowUDPPortsLookup[port] = true
			}
		}

		if len(rules.DisallowTCPPorts) >= intLookupThreshold {
			rules.disallowTCPPortsLookup = make(map[int]bool)
			for _, port := range rules.DisallowTCPPorts {
				rules.disallowTCPPortsLookup[port] = true
			}
		}

		if len(rules.DisallowUDPPorts) >= intLookupThreshold {
			rules.disallowUDPPortsLookup = make(map[int]bool)
			for _, port := range rules.DisallowUDPPorts {
				rules.disallowUDPPortsLookup[port] = true
			}
		}
	}

	initTrafficRulesFilterLookups := func(filter *TrafficRulesFilter) {

		if len(filter.Regions) >= stringLookupThreshold {
			filter.regionLookup = make(map[string]bool)
			for _, region := range filter.Regions {
				filter.regionLookup[region] = true
			}
		}

		if len(filter.ISPs) >= stringLookupThreshold {
			filter.ispLookup = make(map[string]bool)
			for _, ISP := range filter.ISPs {
				filter.ispLookup[ISP] = true
			}
		}

		if len(filter.Cities) >= stringLookupThreshold {
			filter.cityLookup = make(map[string]bool)
			for _, city := range filter.Cities {
				filter.cityLookup[city] = true
			}
		}

		if len(filter.ActiveAuthorizationIDs) >= stringLookupThreshold {
			filter.activeAuthorizationIDLookup = make(map[string]bool)
			for _, ID := range filter.ActiveAuthorizationIDs {
				filter.activeAuthorizationIDLookup[ID] = true
			}
		}
	}

	initTrafficRulesLookups(&set.DefaultRules)

	for i := range set.FilteredRules {
		initTrafficRulesFilterLookups(&set.FilteredRules[i].Filter)
		initTrafficRulesLookups(&set.FilteredRules[i].Rules)
	}

	// TODO: add lookups for MeekRateLimiter?
}

// GetTrafficRules determines the traffic rules for a client based on its attributes.
// For the return value TrafficRules, all pointer and slice fields are initialized,
// so nil checks are not required. The caller must not modify the returned TrafficRules.
func (set *TrafficRulesSet) GetTrafficRules(
	isFirstTunnelInSession bool,
	tunnelProtocol string,
	geoIPData GeoIPData,
	state handshakeState) TrafficRules {

	set.ReloadableFile.RLock()
	defer set.ReloadableFile.RUnlock()

	// Start with a copy of the DefaultRules, and then select the first
	// matching Rules from FilteredTrafficRules, taking only the explicitly
	// specified fields from that Rules.
	//
	// Notes:
	// - Scalar pointers are used in TrafficRules and RateLimits to distinguish between
	//   omitted fields (in serialized JSON) and default values. For example, if a filtered
	//   Rules specifies a field value of 0, this will override the default; but if the
	//   serialized filtered rule omits the field, the default is to be retained.
	// - We use shallow copies and slices and scalar pointers are shared between the
	//   return value TrafficRules, so callers must treat the return value as immutable.
	//   This also means that these slices and pointers can remain referenced in memory even
	//   after a hot reload.

	trafficRules := set.DefaultRules

	// Populate defaults for omitted DefaultRules fields

	if trafficRules.RateLimits.ReadUnthrottledBytes == nil {
		trafficRules.RateLimits.ReadUnthrottledBytes = new(int64)
	}

	if trafficRules.RateLimits.ReadBytesPerSecond == nil {
		trafficRules.RateLimits.ReadBytesPerSecond = new(int64)
	}

	if trafficRules.RateLimits.WriteUnthrottledBytes == nil {
		trafficRules.RateLimits.WriteUnthrottledBytes = new(int64)
	}

	if trafficRules.RateLimits.WriteBytesPerSecond == nil {
		trafficRules.RateLimits.WriteBytesPerSecond = new(int64)
	}

	if trafficRules.RateLimits.CloseAfterExhausted == nil {
		trafficRules.RateLimits.CloseAfterExhausted = new(bool)
	}

	if trafficRules.RateLimits.UnthrottleFirstTunnelOnly == nil {
		trafficRules.RateLimits.UnthrottleFirstTunnelOnly = new(bool)
	}

	intPtr := func(i int) *int {
		return &i
	}

	if trafficRules.DialTCPPortForwardTimeoutMilliseconds == nil {
		trafficRules.DialTCPPortForwardTimeoutMilliseconds =
			intPtr(DEFAULT_DIAL_TCP_PORT_FORWARD_TIMEOUT_MILLISECONDS)
	}

	if trafficRules.IdleTCPPortForwardTimeoutMilliseconds == nil {
		trafficRules.IdleTCPPortForwardTimeoutMilliseconds =
			intPtr(DEFAULT_IDLE_TCP_PORT_FORWARD_TIMEOUT_MILLISECONDS)
	}

	if trafficRules.IdleUDPPortForwardTimeoutMilliseconds == nil {
		trafficRules.IdleUDPPortForwardTimeoutMilliseconds =
			intPtr(DEFAULT_IDLE_UDP_PORT_FORWARD_TIMEOUT_MILLISECONDS)
	}

	if trafficRules.MaxTCPDialingPortForwardCount == nil {
		trafficRules.MaxTCPDialingPortForwardCount =
			intPtr(DEFAULT_MAX_TCP_DIALING_PORT_FORWARD_COUNT)
	}

	if trafficRules.MaxTCPPortForwardCount == nil {
		trafficRules.MaxTCPPortForwardCount =
			intPtr(DEFAULT_MAX_TCP_PORT_FORWARD_COUNT)
	}

	if trafficRules.MaxUDPPortForwardCount == nil {
		trafficRules.MaxUDPPortForwardCount =
			intPtr(DEFAULT_MAX_UDP_PORT_FORWARD_COUNT)
	}

	if trafficRules.AllowTCPPorts == nil {
		trafficRules.AllowTCPPorts = make([]int, 0)
	}

	if trafficRules.AllowUDPPorts == nil {
		trafficRules.AllowUDPPorts = make([]int, 0)
	}

	if trafficRules.AllowSubnets == nil {
		trafficRules.AllowSubnets = make([]string, 0)
	}

	// TODO: faster lookup?
	for _, filteredRules := range set.FilteredRules {

		log.WithTraceFields(LogFields{"filter": filteredRules.Filter}).Debug("filter check")

		if len(filteredRules.Filter.TunnelProtocols) > 0 {
			if !common.Contains(filteredRules.Filter.TunnelProtocols, tunnelProtocol) {
				continue
			}
		}

		if len(filteredRules.Filter.Regions) > 0 {
			if filteredRules.Filter.regionLookup != nil {
				if !filteredRules.Filter.regionLookup[geoIPData.Country] {
					continue
				}
			} else {
				if !common.Contains(filteredRules.Filter.Regions, geoIPData.Country) {
					continue
				}
			}
		}

		if len(filteredRules.Filter.ISPs) > 0 {
			if filteredRules.Filter.ispLookup != nil {
				if !filteredRules.Filter.ispLookup[geoIPData.ISP] {
					continue
				}
			} else {
				if !common.Contains(filteredRules.Filter.ISPs, geoIPData.ISP) {
					continue
				}
			}
		}

		if len(filteredRules.Filter.Cities) > 0 {
			if filteredRules.Filter.cityLookup != nil {
				if !filteredRules.Filter.cityLookup[geoIPData.City] {
					continue
				}
			} else {
				if !common.Contains(filteredRules.Filter.Cities, geoIPData.City) {
					continue
				}
			}
		}

		if filteredRules.Filter.APIProtocol != "" {
			if !state.completed {
				continue
			}
			if state.apiProtocol != filteredRules.Filter.APIProtocol {
				continue
			}
		}

		if filteredRules.Filter.HandshakeParameters != nil {
			if !state.completed {
				continue
			}

			mismatch := false
			for name, values := range filteredRules.Filter.HandshakeParameters {
				clientValue, err := getStringRequestParam(state.apiParams, name)
				if err != nil || !common.ContainsWildcard(values, clientValue) {
					mismatch = true
					break
				}
			}
			if mismatch {
				continue
			}
		}

		if filteredRules.Filter.AuthorizationsRevoked {
			if !state.completed {
				continue
			}

			if !state.authorizationsRevoked {
				continue
			}

		} else {
			if len(filteredRules.Filter.ActiveAuthorizationIDs) > 0 {
				if !state.completed {
					continue
				}

				if state.authorizationsRevoked {
					continue
				}

				if filteredRules.Filter.activeAuthorizationIDLookup != nil {
					found := false
					for _, ID := range state.activeAuthorizationIDs {
						if filteredRules.Filter.activeAuthorizationIDLookup[ID] {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				} else {
					if !common.ContainsAny(filteredRules.Filter.ActiveAuthorizationIDs, state.activeAuthorizationIDs) {
						continue
					}
				}

			}
			if len(filteredRules.Filter.AuthorizedAccessTypes) > 0 {
				if !state.completed {
					continue
				}

				if state.authorizationsRevoked {
					continue
				}

				if !common.ContainsAny(filteredRules.Filter.AuthorizedAccessTypes, state.authorizedAccessTypes) {
					continue
				}
			}
		}

		log.WithTraceFields(LogFields{"filter": filteredRules.Filter}).Debug("filter match")

		// This is the first match. Override defaults using provided fields from selected rules, and return result.

		if filteredRules.Rules.RateLimits.ReadUnthrottledBytes != nil {
			trafficRules.RateLimits.ReadUnthrottledBytes = filteredRules.Rules.RateLimits.ReadUnthrottledBytes
		}

		if filteredRules.Rules.RateLimits.ReadBytesPerSecond != nil {
			trafficRules.RateLimits.ReadBytesPerSecond = filteredRules.Rules.RateLimits.ReadBytesPerSecond
		}

		if filteredRules.Rules.RateLimits.WriteUnthrottledBytes != nil {
			trafficRules.RateLimits.WriteUnthrottledBytes = filteredRules.Rules.RateLimits.WriteUnthrottledBytes
		}

		if filteredRules.Rules.RateLimits.WriteBytesPerSecond != nil {
			trafficRules.RateLimits.WriteBytesPerSecond = filteredRules.Rules.RateLimits.WriteBytesPerSecond
		}

		if filteredRules.Rules.RateLimits.CloseAfterExhausted != nil {
			trafficRules.RateLimits.CloseAfterExhausted = filteredRules.Rules.RateLimits.CloseAfterExhausted
		}

		if filteredRules.Rules.RateLimits.UnthrottleFirstTunnelOnly != nil {
			trafficRules.RateLimits.UnthrottleFirstTunnelOnly = filteredRules.Rules.RateLimits.UnthrottleFirstTunnelOnly
		}

		if filteredRules.Rules.DialTCPPortForwardTimeoutMilliseconds != nil {
			trafficRules.DialTCPPortForwardTimeoutMilliseconds = filteredRules.Rules.DialTCPPortForwardTimeoutMilliseconds
		}

		if filteredRules.Rules.IdleTCPPortForwardTimeoutMilliseconds != nil {
			trafficRules.IdleTCPPortForwardTimeoutMilliseconds = filteredRules.Rules.IdleTCPPortForwardTimeoutMilliseconds
		}

		if filteredRules.Rules.IdleUDPPortForwardTimeoutMilliseconds != nil {
			trafficRules.IdleUDPPortForwardTimeoutMilliseconds = filteredRules.Rules.IdleUDPPortForwardTimeoutMilliseconds
		}

		if filteredRules.Rules.MaxTCPDialingPortForwardCount != nil {
			trafficRules.MaxTCPDialingPortForwardCount = filteredRules.Rules.MaxTCPDialingPortForwardCount
		}

		if filteredRules.Rules.MaxTCPPortForwardCount != nil {
			trafficRules.MaxTCPPortForwardCount = filteredRules.Rules.MaxTCPPortForwardCount
		}

		if filteredRules.Rules.MaxUDPPortForwardCount != nil {
			trafficRules.MaxUDPPortForwardCount = filteredRules.Rules.MaxUDPPortForwardCount
		}

		if filteredRules.Rules.AllowTCPPorts != nil {
			trafficRules.AllowTCPPorts = filteredRules.Rules.AllowTCPPorts
			trafficRules.allowTCPPortsLookup = filteredRules.Rules.allowTCPPortsLookup
		}

		if filteredRules.Rules.AllowUDPPorts != nil {
			trafficRules.AllowUDPPorts = filteredRules.Rules.AllowUDPPorts
			trafficRules.allowUDPPortsLookup = filteredRules.Rules.allowUDPPortsLookup
		}

		if filteredRules.Rules.DisallowTCPPorts != nil {
			trafficRules.DisallowTCPPorts = filteredRules.Rules.DisallowTCPPorts
			trafficRules.disallowTCPPortsLookup = filteredRules.Rules.disallowTCPPortsLookup
		}

		if filteredRules.Rules.DisallowUDPPorts != nil {
			trafficRules.DisallowUDPPorts = filteredRules.Rules.DisallowUDPPorts
			trafficRules.disallowUDPPortsLookup = filteredRules.Rules.disallowUDPPortsLookup
		}

		if filteredRules.Rules.AllowSubnets != nil {
			trafficRules.AllowSubnets = filteredRules.Rules.AllowSubnets
		}

		break
	}

	if *trafficRules.RateLimits.UnthrottleFirstTunnelOnly && !isFirstTunnelInSession {
		trafficRules.RateLimits.ReadUnthrottledBytes = new(int64)
		trafficRules.RateLimits.WriteUnthrottledBytes = new(int64)
	}

	log.WithTraceFields(LogFields{"trafficRules": trafficRules}).Debug("selected traffic rules")

	return trafficRules
}

func (rules *TrafficRules) AllowTCPPort(remoteIP net.IP, port int) bool {

	if len(rules.DisallowTCPPorts) > 0 {
		if rules.disallowTCPPortsLookup != nil {
			if rules.disallowTCPPortsLookup[port] {
				return false
			}
		} else {
			for _, disallowPort := range rules.DisallowTCPPorts {
				if port == disallowPort {
					return false
				}
			}
		}
	}

	if len(rules.AllowTCPPorts) == 0 {
		return true
	}

	if rules.allowTCPPortsLookup != nil {
		if rules.allowTCPPortsLookup[port] {
			return true
		}
	} else {
		for _, allowPort := range rules.AllowTCPPorts {
			if port == allowPort {
				return true
			}
		}
	}

	return rules.allowSubnet(remoteIP)
}

func (rules *TrafficRules) AllowUDPPort(remoteIP net.IP, port int) bool {

	if len(rules.DisallowUDPPorts) > 0 {
		if rules.disallowUDPPortsLookup != nil {
			if rules.disallowUDPPortsLookup[port] {
				return false
			}
		} else {
			for _, disallowPort := range rules.DisallowUDPPorts {
				if port == disallowPort {
					return false
				}
			}
		}
	}

	if len(rules.AllowUDPPorts) == 0 {
		return true
	}

	if rules.allowUDPPortsLookup != nil {
		if rules.allowUDPPortsLookup[port] {
			return true
		}
	} else {
		for _, allowPort := range rules.AllowUDPPorts {
			if port == allowPort {
				return true
			}
		}
	}

	return rules.allowSubnet(remoteIP)
}

func (rules *TrafficRules) allowSubnet(remoteIP net.IP) bool {

	for _, subnet := range rules.AllowSubnets {
		// Note: ignoring error as config has been validated
		_, network, _ := net.ParseCIDR(subnet)
		if network.Contains(remoteIP) {
			return true
		}
	}

	return false
}

// GetMeekRateLimiterConfig gets a snapshot of the meek rate limiter
// configuration values.
func (set *TrafficRulesSet) GetMeekRateLimiterConfig() (int, int, []string, []string, []string, int, int) {

	set.ReloadableFile.RLock()
	defer set.ReloadableFile.RUnlock()

	GCTriggerCount := set.MeekRateLimiterGarbageCollectionTriggerCount
	if GCTriggerCount <= 0 {
		GCTriggerCount = DEFAULT_MEEK_RATE_LIMITER_GARBAGE_COLLECTOR_TRIGGER_COUNT
	}

	reapFrequencySeconds := set.MeekRateLimiterReapHistoryFrequencySeconds
	if reapFrequencySeconds <= 0 {
		reapFrequencySeconds = DEFAULT_MEEK_RATE_LIMITER_REAP_HISTORY_FREQUENCY_SECONDS

	}

	return set.MeekRateLimiterHistorySize,
		set.MeekRateLimiterThresholdSeconds,
		set.MeekRateLimiterRegions,
		set.MeekRateLimiterISPs,
		set.MeekRateLimiterCities,
		GCTriggerCount,
		reapFrequencySeconds
}
