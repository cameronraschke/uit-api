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

type RateLimiter struct {
	Type     string
	Limiter  *rate.Limiter
	LastSeen time.Time
}

func GetGlobalLimiters(limiterType string) *RateLimiter {
	switch limiterType {
	case "file":
		fileLimiterCopy := fileServerRateLimiter
		return &fileLimiterCopy
	case "web":
		webLimiterCopy := webServerRateLimiter
		return &webLimiterCopy
	case "api":
		apiLimiterCopy := apiRateLimiter
		return &apiLimiterCopy
	case "auth":
		authLimiterCopy := authRateLimiter
		return &authLimiterCopy
	default:
		return nil
	}
}

func AttachLimiterToClient(ipAddr netip.Addr, limiterType string) error {
	if ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return errors.New("invalid IP address")
	}

	appState, err := GetAppState()
	if err != nil {
		return errors.New("app state is not initialized")
	}

	checkExistingRL := func(mu *sync.RWMutex, m map[netip.Addr]RateLimiter) bool {
		mu.RLock()
		defer mu.RUnlock()
		if _, exists := m[ipAddr]; exists {
			return true
		}
		return false
	}

	createNewRL := func(mu *sync.RWMutex, m map[netip.Addr]RateLimiter, l *RateLimiter) *RateLimiter {
		newCrl := new(RateLimiter)
		newCrl.LastSeen = time.Now()
		newCrl.Limiter = l.Limiter
		mu.Lock()
		defer mu.Unlock()
		m[ipAddr] = *newCrl
		return newCrl
	}

	var limiter *RateLimiter
	switch limiterType {
	case "file":
		if checkExistingRL(&appState.fileLimiterMu, appState.fileLimiterMap) {
			return nil // Already exists, no need to create a new one
		}
		limiter = &fileServerRateLimiter
		limiter = createNewRL(&appState.fileLimiterMu, appState.fileLimiterMap, limiter)
	case "web":
		if checkExistingRL(&appState.webServerLimiterMu, appState.webServerLimiterMap) {
			return nil // Already exists, no need to create a new one
		}
		limiter = &webServerRateLimiter
		limiter = createNewRL(&appState.webServerLimiterMu, appState.webServerLimiterMap, limiter)
	case "api":
		if checkExistingRL(&appState.apiLimiterMu, appState.apiLimiterMap) {
			return nil // Already exists, no need to create a new one
		}
		limiter = &apiRateLimiter
		limiter = createNewRL(&appState.apiLimiterMu, appState.apiLimiterMap, limiter)
	case "auth":
		if checkExistingRL(&appState.authLimiterMu, appState.authLimiterMap) {
			return nil // Already exists, no need to create a new one
		}
		limiter = &authRateLimiter
		limiter = createNewRL(&appState.authLimiterMu, appState.authLimiterMap, limiter)
	default:
		return errors.New("invalid limiter type")
	}

	return nil
}

func GetClientLimiter(ipAddr netip.Addr, limiterType string) *rate.Limiter {
	defaultLimiter := rate.NewLimiter(apiRateLimiter.Limiter.Limit(), apiRateLimitBurst) // Default rate limit if not found
	if ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return defaultLimiter
	}

	as, err := GetAppState()
	if err != nil {
		return defaultLimiter
	}

	var limiter RateLimiter
	switch limiterType {
	case "file":
		as.fileLimiterMu.RLock()
		limiter, _ = as.fileLimiterMap[ipAddr]
		as.fileLimiterMu.RUnlock()
	case "web":
		as.webServerLimiterMu.RLock()
		limiter, _ = as.webServerLimiterMap[ipAddr]
		as.webServerLimiterMu.RUnlock()
	case "api":
		as.apiLimiterMu.RLock()
		limiter, _ = as.apiLimiterMap[ipAddr]
		as.apiLimiterMu.RUnlock()
	case "auth":
		as.authLimiterMu.RLock()
		limiter, _ = as.authLimiterMap[ipAddr]
		as.authLimiterMu.RUnlock()
	default:
		return defaultLimiter
	}

	if limiter.Limiter == nil {
		return defaultLimiter
	}
	return limiter.Limiter
}

func BlockIP(ip netip.Addr) {
	appState, err := GetAppState()
	if err != nil || appState == nil {
		return
	}
	appState.bannedClientsMu.Lock()
	defer appState.bannedClientsMu.Unlock()
	if _, exists := appState.bannedClients[ip]; !exists {
		appState.bannedClients[ip] = bannedUntil()
	}
}

func IsClientRateLimited(ipAddr netip.Addr, limiterType string) (limited bool, bannedUntil time.Time) {
	appState, err := GetAppState()
	if err != nil || ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return false, bannedUntil
	}

	// Check if IP is currently blocked
	blocked := IsIPBlocked(ipAddr)
	if blocked {
		appState.bannedClientsMu.Lock()
		defer appState.bannedClientsMu.Unlock()
		if bannedUntil, exists := appState.bannedClients[ipAddr]; exists {
			if time.Now().After(bannedUntil) {
				delete(appState.bannedClients, ipAddr) // Remove from banned list after ban expires
				return false, bannedUntil
			}
			return true, bannedUntil
		}
	}

	limiter := GetClientLimiter(ipAddr, limiterType)

	// Use Allow() to check if the request can proceed immediately.
	// If it returns false, the rate limit has been exceeded.
	if !limiter.Allow() {
		appState.bannedClientsMu.Lock()
		defer appState.bannedClientsMu.Unlock()
		appState.bannedClients[ipAddr] = bannedUntil

		return true, bannedUntil
	}

	return false, time.Time{}
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
