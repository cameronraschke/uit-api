package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
	"uit-api/appstate"
	"uit-api/config"
	"uit-api/logger"
	"uit-api/types"
)

var (
	AuthSessions = new(types.AuthSessionsMap)
)

func InitAuthSessions() error {
	AuthSessions.M = make(map[string]types.AuthSession, 100)
	AuthSessions.Mu = new(sync.RWMutex)
	return nil
}

func GetAuthSessionsCopy() map[string]types.AuthSession {
	AuthSessions.Mu.RLock()
	defer AuthSessions.Mu.RUnlock()
	authMapCopy := make(map[string]types.AuthSession, len(AuthSessions.M))
	for sessionID, value := range AuthSessions.M {
		authMapCopy[sessionID] = value
	}
	return authMapCopy
}

func GetAuthSessionsPtr() (map[string]types.AuthSession, *sync.RWMutex) {
	return AuthSessions.M, AuthSessions.Mu
}

// Auth for web users
func GetAdminCredentials() (string, string, error) {
	ac, err := config.GetAppConfig()
	if err != nil {
		return "", "", fmt.Errorf("error getting app state in GetAdminCredentials: %w", err)
	}

	adminUsername := "admin"
	adminPasswd := ac.WebUserDefaultPasswd
	return adminUsername, adminPasswd, nil
}

func CreateAuthSession(requestIP netip.Addr) (*types.AuthSession, error) {
	if requestIP == (netip.Addr{}) || !requestIP.IsValid() {
		return nil, errors.New("empty or invalid IP address")
	}
	sessions, mu := GetAuthSessionsPtr()

	mu.RLock()
	if len(sessions) >= 1000 {
		mu.RUnlock()
		return nil, types.ErrTooManyAuthSessions
	}
	mu.RUnlock()

	curTime := time.Now()

	sessionID := rand.Text()
	basicToken := rand.Text()
	bearerToken := rand.Text()
	csrfToken := rand.Text()

	authSession := types.AuthSession{
		IPAddress:  requestIP,
		SessionID:  sessionID,
		SessionTTL: types.AuthSessionTTL,
		SessionCookie: &http.Cookie{
			Name:     "uit_session_id",
			Value:    sessionID,
			Path:     "/",
			Expires:  curTime.Add(types.AuthSessionTTL),
			MaxAge:   int(types.AuthSessionTTL.Seconds()),
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
		BasicToken: types.BasicToken{
			Token:     basicToken,
			Expiry:    curTime.Add(types.BasicTTL),
			NotBefore: curTime,
			TTL:       types.BasicTTL,
			IP:        requestIP,
			Valid:     true,
		},
		BasicCookie: &http.Cookie{
			Name:     "uit_basic_token",
			Value:    basicToken,
			Path:     "/",
			Expires:  curTime.Add(types.BasicTTL),
			MaxAge:   int(types.BasicTTL.Seconds()),
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
		BearerToken: types.BearerToken{
			Token:     bearerToken,
			Expiry:    curTime.Add(types.BearerTTL),
			NotBefore: curTime,
			TTL:       types.BearerTTL,
			IP:        requestIP,
			Valid:     true,
		},
		BearerCookie: &http.Cookie{
			Name:     "uit_bearer_token",
			Value:    bearerToken,
			Path:     "/",
			Expires:  curTime.Add(types.BearerTTL),
			MaxAge:   int(types.BearerTTL.Seconds()),
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
		CSRFToken: types.CSRFToken{
			Token:     csrfToken,
			Expiry:    curTime.Add(types.CSRFTTL),
			NotBefore: curTime,
			TTL:       types.CSRFTTL,
			IP:        requestIP,
			Valid:     true,
		},
		CSRFCookie: &http.Cookie{
			Name:     "uit_csrf_token",
			Value:    csrfToken,
			Path:     "/",
			Expires:  curTime.Add(types.CSRFTTL),
			MaxAge:   int(types.CSRFTTL.Seconds()),
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
	}

	mu.Lock()
	defer mu.Unlock()
	sessions[authSession.SessionID] = authSession

	return &authSession, nil
}

func DeleteAuthSessions(sessionIDs []string) []error {
	errSlice := make([]error, 0, len(sessionIDs))

	log := logger.GetLogger()
	sessions, mu := GetAuthSessionsPtr()

	stringsToLog := make([]string, 0, len(sessionIDs))

	mu.Lock()
	for _, sessionID := range sessionIDs {
		if _, ok := sessions[sessionID]; ok {
			ipAddress := sessions[sessionID].IPAddress.String()
			delete(sessions, sessionID)
			sessionCount := len(sessions)
			stringsToLog = append(stringsToLog, "Deleted auth session with ID: "+sessionID+" (IP: "+ipAddress+", active sessions: "+strconv.Itoa(sessionCount)+")")
		} else {
			errSlice = append(errSlice, fmt.Errorf("Attempted to delete non-existent auth session with ID: %s", sessionID))
		}
	}
	mu.Unlock()

	for _, msg := range stringsToLog {
		log.Infof("%s", msg)
	}
	return errSlice
}

func ClearExpiredAuthSessions() {
	log := logger.GetLogger()
	sessions, mu := GetAuthSessionsPtr()
	curTime := time.Now()

	expiredAuthSessions := make([]string, 0, 10)
	stringsToLog := make([]string, 0, 10)

	mu.RLock()
	for sessionID, authSession := range sessions {
		if authSession.BasicToken.Expiry.Before(curTime) &&
			authSession.BearerToken.Expiry.Before(curTime) {
			expiredAuthSessions = append(expiredAuthSessions, sessionID)
			stringsToLog = append(stringsToLog, "Auth session expired: "+authSession.BasicToken.IP.String()+" (TTL: "+fmt.Sprintf("%.2f", authSession.BearerToken.Expiry.Sub(curTime).Seconds())+")")
		}
	}
	mu.RUnlock()

	for _, msg := range stringsToLog {
		log.Infof("%s", msg)
	}

	if errSlice := DeleteAuthSessions(expiredAuthSessions); len(errSlice) > 0 {
		for _, err := range errSlice {
			log.Warnf("%v", err)
		}
	}
}

func GetAuthSessionCount() int {
	am := GetAuthSessionsCopy()
	return len(am)
}

func IsAuthSessionValid(checkedAuthSession *types.AuthSession, requestIP netip.Addr) (bool, error) {
	curTime := time.Now()

	if checkedAuthSession == nil || checkedAuthSession.SessionID == "" || requestIP == (netip.Addr{}) || !requestIP.IsValid() {
		return false, fmt.Errorf("auth session and/or request IP is nil or invalid (IsAuthSessionValid)")
	}

	authSession, err := GetAuthSessionByID(checkedAuthSession.SessionID)
	if err != nil {
		return false, fmt.Errorf("error retrieving auth session by ID: %w", err)
	}

	if authSession.SessionTTL <= 0 ||
		authSession.BasicToken.TTL <= 0 ||
		authSession.BearerToken.TTL <= 0 {
		// authSession.CSRFToken.TTL <= 0
		return false, fmt.Errorf("auth tokens have reached their TTL")
	}

	if authSession.SessionID != checkedAuthSession.SessionID ||
		authSession.BasicToken.Token != checkedAuthSession.BasicToken.Token ||
		authSession.BearerToken.Token != checkedAuthSession.BearerToken.Token {
		// authSession.CSRFToken.Token != checkedAuthSession.CSRFToken.Token
		return false, fmt.Errorf("request tokens do not match stored session tokens")
	}

	if authSession.BasicToken.Expiry.Before(curTime) ||
		authSession.BearerToken.Expiry.Before(curTime) {
		// authSession.CSRFToken.Expiry.Before(curTime)
		return false, fmt.Errorf("auth tokens have expired")
	}

	if authSession.IPAddress != requestIP ||
		authSession.BasicToken.IP != requestIP ||
		authSession.BearerToken.IP != requestIP {
		// authSession.CSRFToken.IP != requestIP
		return false, fmt.Errorf("request IP does not match stored token IP")
	}

	return true, nil
}

func GetAuthSessionByID(sessionID string) (*types.AuthSession, error) {
	sessionsCopy := GetAuthSessionsCopy()

	authSession, ok := sessionsCopy[sessionID]
	if !ok {
		return nil, fmt.Errorf("auth session not found")
	}
	return &authSession, nil
}

func UpdateAuthSession(sessionID string, newAuthSession *types.AuthSession) error {
	sessions, mu := GetAuthSessionsPtr()

	if newAuthSession == nil || newAuthSession.SessionID == "" {
		return fmt.Errorf("new auth session is nil or has empty session ID")
	}

	mu.Lock()
	defer mu.Unlock()
	if authSession, ok := sessions[sessionID]; !ok || authSession.SessionID == "" {
		return fmt.Errorf("auth session not found for session ID: %s", sessionID)
	}

	sessions[sessionID] = *newAuthSession
	return nil
}

// SignSessionToken returns HMAC-SHA256(token) using a server-side secret key.
func SignSessionToken(clientToken string, serverSecret []byte) (string, error) {
	hmacHash := hmac.New(sha256.New, serverSecret)
	hmacHash.Write([]byte(clientToken))
	return hex.EncodeToString(hmacHash.Sum(nil)), nil
}

// Check SessionToken by hashing the client token and comparing to server-side hash
func IsSessionTokenValid(clientToken string, serverSecret []byte) bool {
	hmacHash := hmac.New(sha256.New, serverSecret)
	hmacHash.Write([]byte(clientToken))
	computedHash := hmacHash.Sum(nil)
	expectedHash, err := SignSessionToken(clientToken, serverSecret)
	if err != nil {
		return false
	}
	return hmac.Equal(computedHash, []byte(expectedHash))
}

func GetServerSecret() ([]byte, error) {
	appState, err := appstate.GetAppState()
	if err != nil {
		return nil, fmt.Errorf("error getting app state in GetServerSecret: %w", err)
	}
	return appState.SessionSecret, nil
}

// IP address checks
func IsIPAllowed(networkDomain string, limiterType string, ipAddr netip.Addr) (allowed bool, retryAt time.Time, err error) {
	ac, err := config.GetAppConfig()
	if err != nil {
		return allowed, retryAt, fmt.Errorf("%w: %w", types.NilAppConfigError, err)
	}
	if !ipAddr.IsValid() {
		return allowed, retryAt, fmt.Errorf("request IP is invalid or blocked")
	}

	var lt LimiterType
	if strings.TrimSpace(limiterType) != "" {
		lt = ToLimiterType(limiterType)
		if !lt.IsValid() {
			return true, retryAt, fmt.Errorf("invalid limiter type: %s", limiterType)
		}
	}

	switch strings.TrimSpace(strings.ToLower(networkDomain)) {
	case "wan":
		allowed, err = ipAllowedInRanges(ipAddr, &ac.AllowedWANMap)
		if err != nil {
			return allowed, retryAt, fmt.Errorf("error checking WAN IP ranges: %w", err)
		}
	case "lan":
		allowed, err = ipAllowedInRanges(ipAddr, &ac.AllowedLANMap)
		if err != nil {
			return allowed, retryAt, fmt.Errorf("error checking LAN IP ranges: %w", err)
		}
	case "any":
		allowed, err = ipAllowedInRanges(ipAddr, &ac.AllAllowedMap)
		if err != nil {
			return allowed, retryAt, fmt.Errorf("error checking IP ranges: %w", err)
		}
	default:
		return allowed, retryAt, errors.New("invalid traffic type, must be 'wan', 'lan', or 'any'")
	}

	if lt.IsValid() {
		isRateLimited, bannedUntil := lt.IsClientRateLimited(ipAddr)
		if isRateLimited {
			return false, bannedUntil, fmt.Errorf("client is rate limited until %s", bannedUntil.Format(time.RFC3339))
		}
	}
	return allowed, retryAt, nil
}

func ipAllowedInRanges(ipAddr netip.Addr, ranges *sync.Map) (allowed bool, err error) {
	if ranges == nil {
		return false, fmt.Errorf("IP range map is nil")
	}

	allowed = false
	ranges.Range(func(k, v any) bool {
		ipRangePtr, ok := k.(*netip.Prefix)
		if !ok || ipRangePtr == nil {
			return true
		}
		ipRange := *ipRangePtr
		if ipRange == (netip.Prefix{}) {
			return true
		}
		if ipRange.Contains(ipAddr) {
			allowed = true
			return false
		}
		return true
	})
	return allowed, nil
}
