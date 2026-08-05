package auth

import (
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
	"uit-api/appstate"
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

type rateLimiterMap struct {
	mu *sync.RWMutex
	m  map[netip.Addr]types.RateLimiter
}

type LimiterType int

const (
	InvalidLimiter LimiterType = iota
	APILimiter
	AuthLimiter
	FileLimiter
	WebLimiter
)

var (
	rateLimiterInstance   atomic.Pointer[types.RequestLimiters]
	webServerRateLimiter  types.RateLimiter
	apiRateLimiter        types.RateLimiter
	authRateLimiter       types.RateLimiter
	fileServerRateLimiter types.RateLimiter
)

func InitRateLimiters() error {
	limiter := new(types.RequestLimiters)

	// Init maps and mutexes for each limiter type
	limiter.APILimiterMap = make(map[netip.Addr]types.RateLimiter, 100)
	limiter.AuthLimiterMap = make(map[netip.Addr]types.RateLimiter, 100)
	limiter.FileLimiterMap = make(map[netip.Addr]types.RateLimiter, 100)
	limiter.WebServerLimiterMap = make(map[netip.Addr]types.RateLimiter, 100)
	limiter.WebServerLimiterMu = sync.RWMutex{}
	limiter.APILimiterMu = sync.RWMutex{}
	limiter.AuthLimiterMu = sync.RWMutex{}
	limiter.FileLimiterMu = sync.RWMutex{}

	limiter.BannedClients = make(map[netip.Addr]time.Time, 100)
	limiter.BannedClientsMu = sync.RWMutex{}

	// Default web server rate limiter
	webServerRateLimiter = types.RateLimiter{
		Type:     "web",
		Limiter:  rate.NewLimiter(rate.Limit(webServerRateLimitInterval), webServerRateLimitBurst),
		LastSeen: time.Time{},
	}

	// Default API rate limiter
	apiRateLimiter = types.RateLimiter{
		Type:     "api",
		Limiter:  rate.NewLimiter(rate.Limit(apiRateLimitInterval), apiRateLimitBurst),
		LastSeen: time.Time{},
	}

	// Default auth rate limiter
	authRateLimiter = types.RateLimiter{
		Type:     "auth",
		Limiter:  rate.NewLimiter(rate.Limit(authRateLimitInterval), authRateLimitBurst),
		LastSeen: time.Time{},
	}
	// Default file server rate limiter
	fileServerRateLimiter = types.RateLimiter{
		Type:     "file",
		Limiter:  rate.NewLimiter(rate.Limit(fileRateLimitInterval), fileRateLimitBurst),
		LastSeen: time.Time{},
	}

	rateLimiterInstance.Store(limiter)
	return nil
}

func GetRateLimiters() (*types.RequestLimiters, error) {
	rateLimiters := rateLimiterInstance.Load()
	if rateLimiters == nil {
		return nil, fmt.Errorf("%w", types.NilRateLimitersError)
	}
	return rateLimiters, nil
}

func CleanupBlockedIPs() {
	lm, err := GetRateLimiters()
	if err != nil || lm == nil {
		return
	}

	lm.BannedClientsMu.Lock()
	defer lm.BannedClientsMu.Unlock()

	now := time.Now()
	for ip, bannedUntil := range lm.BannedClients {
		if now.After(bannedUntil) {
			delete(lm.BannedClients, ip)
		}
	}
}

func nextBanExpiry(now time.Time) time.Time {
	banDuration := types.RateLimitTimeout
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
	as, err := appstate.GetAppState()
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

func (lt LimiterType) GetAssociatedMap() (map[netip.Addr]types.RateLimiter, *sync.RWMutex) {
	lm, err := GetRateLimiters()
	if err != nil || lm == nil {
		return nil, nil
	}
	switch lt {
	case APILimiter:
		return lm.APILimiterMap, &lm.APILimiterMu
	case AuthLimiter:
		return lm.AuthLimiterMap, &lm.AuthLimiterMu
	case FileLimiter:
		return lm.FileLimiterMap, &lm.FileLimiterMu
	case WebLimiter:
		return lm.WebServerLimiterMap, &lm.WebServerLimiterMu
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

	limiterMap[ipAddr] = types.RateLimiter{
		Type:     lt.String(),
		Limiter:  newLimiter,
		LastSeen: time.Now(),
	}
	return nil
}

func (lt LimiterType) IsClientRateLimited(ipAddr netip.Addr) (isRateLimited bool, blockedUntil time.Time) {
	lm, err := GetRateLimiters()
	if err != nil || lm == nil || ipAddr == (netip.Addr{}) || !ipAddr.IsValid() || !lt.IsValid() {
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
		lm.BannedClientsMu.Lock()
		lm.BannedClients[ipAddr] = banExpiry
		lm.BannedClientsMu.Unlock()
		return true, banExpiry
	}
	return false, blockedUntil
}

func IsIPBlocked(ipAddr netip.Addr) (isBlocked bool, blockedUntil time.Time) {
	lm, err := GetRateLimiters()
	if err != nil || lm == nil || ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return false, blockedUntil
	}

	lm.BannedClientsMu.Lock()
	defer lm.BannedClientsMu.Unlock()
	now := time.Now()
	if blockedUntil, exists := lm.BannedClients[ipAddr]; exists {
		if !blockedUntil.After(now) {
			delete(lm.BannedClients, ipAddr) // Remove from banned list after ban expires
			return false, blockedUntil
		}
		return true, blockedUntil
	}
	return false, blockedUntil
}

func BlockIP(ip netip.Addr) {
	lm, err := GetRateLimiters()
	if err != nil || lm == nil {
		return
	}
	lm.BannedClientsMu.Lock()
	defer lm.BannedClientsMu.Unlock()
	if _, exists := lm.BannedClients[ip]; !exists {
		lm.BannedClients[ip] = nextBanExpiry(time.Now())
	}
}

func CleanupExpiredBans() (totalDeleted int64, totalEntries int64, err error) {
	lm, err := GetRateLimiters()
	if err != nil || lm == nil {
		return
	}

	now := time.Now()
	lm.BannedClientsMu.Lock()
	defer lm.BannedClientsMu.Unlock()
	for ip, expiry := range lm.BannedClients {
		if !expiry.After(now) {
			delete(lm.BannedClients, ip)
			atomic.AddInt64(&totalDeleted, 1)
		}
	}
	totalEntries = int64(len(lm.BannedClients))
	return totalDeleted, totalEntries, nil
}

func CleanupOldLimiterEntries() (totalDeleted int64, totalEntries int64, err error) {
	lm, err := GetRateLimiters()
	if err != nil || lm == nil {
		return totalDeleted, totalEntries, types.CannotGetAppStateError
	}
	now := time.Now()
	var wg sync.WaitGroup

	allMaps := []rateLimiterMap{
		{mu: &lm.WebServerLimiterMu, m: lm.WebServerLimiterMap},
		{mu: &lm.APILimiterMu, m: lm.APILimiterMap},
		{mu: &lm.AuthLimiterMu, m: lm.AuthLimiterMap},
		{mu: &lm.FileLimiterMu, m: lm.FileLimiterMap},
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
