package config

import (
	"errors"
	"net/netip"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	Type     string
	Limiter  *rate.Limiter
	LastSeen time.Time
}

func GetGlobalLimiters(limiterType string) *RateLimiter {
	appState, err := GetAppState()
	if err != nil {
		return nil
	}

	switch limiterType {
	case "file":
		return appState.fileLimiter.Load()
	case "web":
		return appState.webServerLimiter.Load()
	case "api":
		return appState.apiLimiter.Load()
	case "auth":
		return appState.authLimiter.Load()
	default:
		return nil
	}
}

func (rl *RateLimiter) GetLimiter(ipAddr netip.Addr) *rate.Limiter {
	defaultLimiter := rate.NewLimiter(rate.Limit(1), 5) // Default rate limit if not found
	if ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return defaultLimiter
	}

	rl.ClientMapMu.Lock()
	defer rl.ClientMapMu.Unlock()
	var crl ClientRateLimiter
	var ipExists bool
	if crl, ipExists = rl.ClientMap[ipAddr]; !ipExists {
		return defaultLimiter
	}

	newCrl := new(ClientRateLimiter)
	newCrl.LastSeen = time.Now()
	newCrl.Limiter = crl.Limiter

	rl.ClientMap[ipAddr] = *newCrl

	return newCrl.Limiter
}

func BlockIP(ip netip.Addr) {
	appState, err := GetAppState()
	if err != nil || appState == nil {
		return
	}
	appState.bannedClientsMu.Lock()
	defer appState.bannedClientsMu.Unlock()
	if _, exists := appState.bannedClients[ip]; !exists {
		appState.bannedClients[ip] = banPeriod
	}
}

func IsIPBlocked(ip netip.Addr) bool {
	appState, err := GetAppState()
	if err != nil || appState == nil {
		return false
	}

	appState.bannedClientsMu.RLock()
	defer appState.bannedClientsMu.RUnlock()
	for blockedIP, _ := range appState.bannedClients {
		if blockedIP == ip {
			return true
		}
	}
	return false
}

func IsClientRateLimited(limiterType string, ipAddr netip.Addr) (limited bool, retryAfter time.Duration) {
	appState, err := GetAppState()
	if err != nil || ipAddr == (netip.Addr{}) || !ipAddr.IsValid() {
		return false, 0
	}

	// Check if IP is currently blocked
	blocked := IsIPBlocked(ipAddr)
	if blocked {
		appState.bannedClientsMu.Lock()
		defer appState.bannedClientsMu.Unlock()
		if banDuration, exists := appState.bannedClients[ipAddr]; exists {
			if curTime := time.Now(); curTime.Before(curTime.Add(banDuration)) {
				delete(appState.bannedClients, ipAddr) // Remove from banned list after ban expires
				return true, banDuration
			}
			return true, banDuration
		}
	}

	limiter := GetLimiter(ipAddr)

	// Use Allow() to check if the request can proceed immediately.
	// If it returns false, the rate limit has been exceeded.
	if !limiter.Allow() {
		appState.banList.Load().Block(ipAddr)
		return true, appState.banList.Load().banPeriod
	}

	return false, 0
}

func CleanupOldLimiterEntries() (int64, error) {
	appState, err := GetAppState()
	if err != nil {
		return 0, errors.New("app state is not initialized")
	}
	now := time.Now()

	var count int
	// Clean up webServerLimiter
	appState.webServerLimiter.Load().ClientMap.Range(func(key, value any) bool {
		clientLimiter, ok := value.(ClientRateLimiter)
		if !ok {
			return true
		}
		if now.Sub(clientLimiter.LastSeen) > 3*time.Minute {
			appState.webServerLimiter.Load().ClientMap.Delete(key)
			count++
		}
		return true
	})
	// File server limiter
	appState.fileLimiter.Load().ClientMap.Range(func(key, value any) bool {
		clientLimiter, ok := value.(ClientRateLimiter)
		if !ok {
			return true
		}
		if now.Sub(clientLimiter.LastSeen) > 3*time.Minute {
			appState.fileLimiter.Load().ClientMap.Delete(key)
			count++
		}
		return true
	})
	return int64(count), nil
}
