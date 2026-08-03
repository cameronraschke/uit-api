package config

import (
	"errors"
	"net/netip"
	"sync"
	"time"
	"uit-api/types"

	"golang.org/x/time/rate"
)

const (
	limitersCleanupInterval = 3 * time.Minute
)

const (
	InvalidLimiter = iota
	APILimiter
	AuthLimiter
	FileLimiter
	WebLimiter
)

type LimiterType int

type RateLimiter struct {
	Type     string
	Limiter  *rate.Limiter
	LastSeen time.Time
}

func nextBanExpiry(now time.Time) time.Time {
	banDuration := rateLimitBanDuration
	if banDuration <= 0 {
		banDuration = time.Minute // Default ban duration if not set
	}
	return now.Add(banDuration)
}

func (lt LimiterType) newLimiter() *rate.Limiter {
	switch lt {
	case APILimiter:
		return rate.NewLimiter(rate.Limit(apiRateLimitInterval), apiRateLimitBurst)
	case AuthLimiter:
		return rate.NewLimiter(rate.Limit(authRateLimitInterval), authRateLimitBurst)
	case FileLimiter:
		return rate.NewLimiter(rate.Limit(fileRateLimitInterval), fileRateLimitBurst)
	case WebLimiter:
		return rate.NewLimiter(rate.Limit(webServerRateLimitInterval), webServerRateLimitBurst)
	default:
		return nil
	}
}

func (lt LimiterType) GetRateLimiterForIP(ipAddr netip.Addr) *rate.Limiter {
	as, err := GetAppState()
	if err != nil || as == nil {
		return nil
	}
	if ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return nil
	}
	if !lt.IsValid() {
		return nil
	}

	if limiterMap, mu := lt.GetAssociatedMap(); limiterMap != nil && mu != nil {
		mu.RLock()
		defer mu.RUnlock()
		if rl, exists := limiterMap[ipAddr]; exists {
			return rl.Limiter
		}
	}
	return nil
}

func ToLimiterType(ls string) (lt LimiterType) {
	switch ls {
	case "api":
		return APILimiter
	case "auth":
		return AuthLimiter
	case "file":
		return FileLimiter
	case "web":
		return WebLimiter
	default:
		return InvalidLimiter
	}
}

func (lt LimiterType) GetAssociatedMap() (map[netip.Addr]RateLimiter, *sync.RWMutex) {
	as, err := GetAppState()
	if err != nil || as == nil {
		return nil, nil
	}
	switch lt {
	case APILimiter:
		return as.apiLimiterMap, &as.apiLimiterMu
	case AuthLimiter:
		return as.authLimiterMap, &as.authLimiterMu
	case FileLimiter:
		return as.fileLimiterMap, &as.fileLimiterMu
	case WebLimiter:
		return as.webServerLimiterMap, &as.webServerLimiterMu
	default:
		return nil, nil
	}
}

func (lt LimiterType) String() string {
	switch lt {
	case APILimiter:
		return "api"
	case AuthLimiter:
		return "auth"
	case FileLimiter:
		return "file"
	case WebLimiter:
		return "web"
	default:
		return "invalid"
	}
}

func (lt LimiterType) IsValid() bool {
	return lt != InvalidLimiter
}

func (lt LimiterType) AttachToClientIP(ipAddr netip.Addr) error {
	if ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return errors.New("invalid IP address")
	}

	if !lt.IsValid() {
		return errors.New("invalid limiter type: " + lt.String())
	}

	limiterMap, limiterMu := lt.GetAssociatedMap()
	if limiterMap == nil || limiterMu == nil {
		return errors.New("could not get limiter map for type: " + lt.String())
	}

	limiterMu.Lock()
	defer limiterMu.Unlock()

	if existing, exists := limiterMap[ipAddr]; exists {
		if existing.Limiter == nil {
			existing.Limiter = lt.newLimiter()
		}
		existing.Type = lt.String()
		existing.LastSeen = time.Now()
		limiterMap[ipAddr] = existing
		return nil
	}

	newLimiter := lt.newLimiter()
	if newLimiter == nil {
		return errors.New("failed to create limiter for type: " + lt.String())
	}

	limiterMap[ipAddr] = RateLimiter{
		Type:     lt.String(),
		Limiter:  newLimiter,
		LastSeen: time.Now(),
	}
	return nil
}

func (lt LimiterType) IsClientRateLimited(ipAddr netip.Addr) (isRateLimited bool, blockedUntil time.Time) {
	as, err := GetAppState()
	if err != nil || as == nil || ipAddr == (netip.Addr{}) || !ipAddr.IsValid() || !lt.IsValid() {
		return false, blockedUntil
	}

	// Check if IP is currently blocked
	isBlocked, blockedUntil := IsIPBlocked(ipAddr)
	if isBlocked {
		return true, blockedUntil
	}

	if err := lt.AttachToClientIP(ipAddr); err != nil {
		return false, blockedUntil
	}

	limiterMap, limiterMu := lt.GetAssociatedMap()
	if limiterMap == nil || limiterMu == nil {
		return false, blockedUntil
	}

	limiterMu.Lock()
	ratelimiterEntry, exists := limiterMap[ipAddr]
	if !exists {
		limiterMu.Unlock()
		return false, blockedUntil
	}

	if ratelimiterEntry.Limiter == nil {
		ratelimiterEntry.Limiter = lt.newLimiter()
	}
	ratelimiterEntry.Type = lt.String()
	ratelimiterEntry.LastSeen = time.Now()
	limiterMap[ipAddr] = ratelimiterEntry
	ratelimiter := ratelimiterEntry.Limiter
	limiterMu.Unlock()

	if ratelimiter == nil {
		return false, blockedUntil
	}

	// Use Allow() to check if the request can proceed immediately.
	// If it returns false, the rate limit has been exceeded.
	if !ratelimiter.Allow() {
		now := time.Now()
		banExpiry := nextBanExpiry(now)
		// Default ban
		if !banExpiry.After(now) {
			banExpiry = now.Add(time.Minute)
		}
		as.bannedClientsMu.Lock()
		as.bannedClients[ipAddr] = banExpiry
		as.bannedClientsMu.Unlock()
		return true, banExpiry
	}
	return false, blockedUntil
}

func IsIPBlocked(ipAddr netip.Addr) (isBlocked bool, blockedUntil time.Time) {
	as, err := GetAppState()
	if err != nil || ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return false, blockedUntil
	}

	as.bannedClientsMu.Lock()
	defer as.bannedClientsMu.Unlock()
	now := time.Now()
	if blockedUntil, exists := as.bannedClients[ipAddr]; exists {
		if !blockedUntil.After(now) {
			delete(as.bannedClients, ipAddr) // Remove from banned list after ban expires
			return false, blockedUntil
		}
		return true, blockedUntil
	}
	return false, blockedUntil
}

func BlockIP(ip netip.Addr) {
	appState, err := GetAppState()
	if err != nil || appState == nil {
		return
	}
	appState.bannedClientsMu.Lock()
	defer appState.bannedClientsMu.Unlock()
	if _, exists := appState.bannedClients[ip]; !exists {
		appState.bannedClients[ip] = nextBanExpiry(time.Now())
	}
}

func CleanupOldLimiterEntries() (entriesDeleted int64, err error) {
	as, err := GetAppState()
	if err != nil {
		return entriesDeleted, types.CannotGetAppStateError
	}
	now := time.Now()

	cleanupLimiterMap := func(mu *sync.RWMutex, m map[netip.Addr]RateLimiter) int64 {
		var deleted int64
		mu.Lock()
		defer mu.Unlock()
		for ip, limiter := range m {
			if now.Sub(limiter.LastSeen) > limitersCleanupInterval {
				delete(m, ip)
				deleted++
			}
		}
		return deleted
	}

	entriesDeleted += cleanupLimiterMap(&as.webServerLimiterMu, as.webServerLimiterMap)
	entriesDeleted += cleanupLimiterMap(&as.apiLimiterMu, as.apiLimiterMap)
	entriesDeleted += cleanupLimiterMap(&as.authLimiterMu, as.authLimiterMap)
	entriesDeleted += cleanupLimiterMap(&as.fileLimiterMu, as.fileLimiterMap)

	return entriesDeleted, nil
}
