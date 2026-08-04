package config

import (
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
	"uit-api/types"

	"golang.org/x/time/rate"
)

const (
	limitersCleanupInterval    = 3 * time.Minute
	webServerRateLimitInterval = 25 // requests per second
	webServerRateLimitBurst    = 75 // maximum burst size
	apiRateLimitInterval       = 25 // requests per second
	apiRateLimitBurst          = 75 // maximum burst size
	authRateLimitInterval      = 10 // requests per second
	authRateLimitBurst         = 20 // maximum burst size
	fileRateLimitInterval      = 10 // requests per second
	fileRateLimitBurst         = 30 // maximum burst size
	defaultBanDuration         = 1 * time.Minute
)

const (
	InvalidLimiter LimiterType = iota
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

type mapWithMutex struct {
	mu *sync.RWMutex
	m  map[netip.Addr]RateLimiter
}

func (as *AppState) initRateLimiters() error {
	// Store rate limiters in app state
	as.webServerLimiterMu.Lock()
	as.webServerLimiterMap = make(map[netip.Addr]RateLimiter, 100)
	webServerRateLimiter = RateLimiter{
		Type:     "web",
		Limiter:  rate.NewLimiter(rate.Limit(webServerRateLimitInterval), webServerRateLimitBurst),
		LastSeen: time.Time{},
	}
	as.webServerLimiterMu.Unlock()

	as.apiLimiterMu.Lock()
	as.apiLimiterMap = make(map[netip.Addr]RateLimiter, 100)
	apiRateLimiter = RateLimiter{
		Type:     "api",
		Limiter:  rate.NewLimiter(rate.Limit(apiRateLimitInterval), apiRateLimitBurst),
		LastSeen: time.Time{},
	}
	as.apiLimiterMu.Unlock()

	as.authLimiterMu.Lock()
	as.authLimiterMap = make(map[netip.Addr]RateLimiter, 10)
	authRateLimiter = RateLimiter{
		Type:     "auth",
		Limiter:  rate.NewLimiter(rate.Limit(authRateLimitInterval), authRateLimitBurst),
		LastSeen: time.Time{},
	}
	as.authLimiterMu.Unlock()

	as.fileLimiterMu.Lock()
	as.fileLimiterMap = make(map[netip.Addr]RateLimiter, 10)
	fileServerRateLimiter = RateLimiter{
		Type:     "file",
		Limiter:  rate.NewLimiter(rate.Limit(fileRateLimitInterval), fileRateLimitBurst),
		LastSeen: time.Time{},
	}
	as.fileLimiterMu.Unlock()

	as.bannedClientsMu.Lock()
	as.bannedClients = make(map[netip.Addr]time.Time, 10)
	as.bannedClientsMu.Unlock()
	return nil
}

func nextBanExpiry(now time.Time) time.Time {
	banDuration := rateLimitTimeout
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

func CleanupExpiredBans() (totalDeleted int64, totalEntries int64, err error) {
	as, err := GetAppState()
	if err != nil || as == nil {
		return
	}

	now := time.Now()
	as.bannedClientsMu.Lock()
	defer as.bannedClientsMu.Unlock()
	for ip, expiry := range as.bannedClients {
		if !expiry.After(now) {
			delete(as.bannedClients, ip)
			atomic.AddInt64(&totalDeleted, 1)
		}
	}
	totalEntries = int64(len(as.bannedClients))
	return totalDeleted, totalEntries, nil
}

func CleanupOldLimiterEntries() (totalDeleted int64, totalEntries int64, err error) {
	as, err := GetAppState()
	if err != nil {
		return totalDeleted, totalEntries, types.CannotGetAppStateError
	}
	now := time.Now()
	var wg sync.WaitGroup

	allMaps := []mapWithMutex{
		{mu: &as.webServerLimiterMu, m: as.webServerLimiterMap},
		{mu: &as.apiLimiterMu, m: as.apiLimiterMap},
		{mu: &as.authLimiterMu, m: as.authLimiterMap},
		{mu: &as.fileLimiterMu, m: as.fileLimiterMap},
	}
	for _, val := range allMaps {
		wg.Go(func() {
			val.mu.Lock()
			defer val.mu.Unlock()
			for ip, limiter := range val.m {
				if now.Sub(limiter.LastSeen) > limitersCleanupInterval {
					delete(val.m, ip)
					atomic.AddInt64(&totalDeleted, 1)
				}
				atomic.AddInt64(&totalEntries, 1)
			}
		})
	}
	return totalDeleted, totalEntries, nil
}
