package auth

import (
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
	"uit-api/appstate"
	"uit-api/config"
	"uit-api/types"

	"golang.org/x/time/rate"
)

const (
	limitersCleanupInterval = 3 * time.Minute
	defaultBanDuration      = 1 * time.Minute
)

var (
	apiRateLimitInterval       = 25 // default requests per second, can set later in InitRateLimiters()
	apiRateLimitBurst          = 75 // default maximum burst size, can set later in InitRateLimiters()
	webServerRateLimitInterval int
	webServerRateLimitBurst    int
	authRateLimitInterval      int
	authRateLimitBurst         int
	fileRateLimitInterval      int
	fileRateLimitBurst         int
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
	rateLimiterInstance atomic.Pointer[types.RequestLimiters]
)

func InitRateLimiters() error {
	limiter := new(types.RequestLimiters)

	ac, err := config.GetAppConfig()
	if err != nil {
		return fmt.Errorf("failed to get app config: %w", err)
	}
	
	limiter.TimeoutDuration = ac.RateLimitTimeout
	apiRateLimitInterval = ac.RateLimitInterval
	apiRateLimitBurst = ac.RateLimitBurst
	webServerRateLimitInterval = apiRateLimitInterval
	webServerRateLimitBurst = apiRateLimitBurst
	authRateLimitInterval = apiRateLimitInterval / 4
	authRateLimitBurst = apiRateLimitBurst / 4
	fileRateLimitInterval = apiRateLimitInterval / 2
	fileRateLimitBurst = apiRateLimitBurst / 2

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

func nextBanExpiry(t time.Time) time.Time {
	banDuration := rateLimiterInstance.Load().TimeoutDuration
	// Default to 1 min in case RateLimitTimeout is not set
	if banDuration <= 0 {
		banDuration = defaultBanDuration
	}
	return t.Add(banDuration)
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
	now := time.Now()

	// Check if IP is currently blocked, if so, push the ban expiry back and return true
	isBlocked, blockedUntil := IsIPBlocked(ipAddr)
	if isBlocked {
		banExpiry := nextBanExpiry(now)
		lm.BannedClientsMu.Lock()
		lm.BannedClients[ipAddr] = banExpiry
		lm.BannedClientsMu.Unlock()
		return true, banExpiry
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
	ratelimiterEntry.LastSeen = now
	limiterMap[ipAddr] = ratelimiterEntry
	ratelimiter := ratelimiterEntry.Limiter
	limiterMu.Unlock()

	if ratelimiter == nil {
		return false, blockedUntil
	}

	// Use Allow() to check if the request can proceed immediately.
	// If it returns false, the rate limit has been exceeded.
	if !ratelimiter.Allow() {
		banExpiry := nextBanExpiry(now)
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
		return 0, 0, nil
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
		return 0, 0, nil
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
				} else {
					atomic.AddInt64(&totalEntries, 1)
				}
			}
		})
	}

	wg.Wait()
	return totalDeleted, totalEntries, nil
}
