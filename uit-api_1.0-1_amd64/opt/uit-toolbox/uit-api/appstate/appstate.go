package appstate

import (
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"uit-api/types"
)

var (
	appStateInstance atomic.Pointer[types.AppState]
)

func InitAppState() error {
	appStateCopy := new(types.AppState)

	// Generate server-side secret for HMAC
	sessionSecret := make([]byte, 32)
	if _, err := rand.Read(sessionSecret); err != nil {
		return fmt.Errorf("failed to generate session secret: %w", err)
	}
	appStateCopy.SessionSecret = sessionSecret

	// Initialize live image map
	clientRealtimeDataMap := make(map[int64]types.JobQueueRealtimeData)
	appStateCopy.ClientRealtimeDataMu.Lock()
	appStateCopy.ClientRealtimeData = clientRealtimeDataMap
	appStateCopy.ClientRealtimeDataMu.Unlock()

	// Declare endpoints
	once := new(sync.Once)
	once.Do(func() {
		appStateCopy.AppStateMu.Lock()
		defer appStateCopy.AppStateMu.Unlock()
		appStateInstance.Store(appStateCopy)
	})
	return nil
}

func GetAppState() (*types.AppState, error) {
	appState := appStateInstance.Load()
	if appState == nil {
		return nil, fmt.Errorf("%w", types.NilAppStateError)
	}
	return appState, nil
}
