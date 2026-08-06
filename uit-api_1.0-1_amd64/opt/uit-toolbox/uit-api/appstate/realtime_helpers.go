package appstate

import (
	"fmt"
	"maps"
	"sync/atomic"
	"time"
	"uit-api/types"

	"github.com/google/uuid"
)

func GetLiveImage(tag int64) ([]byte, error) {
	as, err := GetAppState()
	if err != nil || as == nil {
		return nil, fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return nil, types.CreateInvalidFieldError("tagnumber", err)
	}

	as.ClientRealtimeDataMu.RLock()
	if len(as.ClientRealtimeData) == 0 {
		as.ClientRealtimeDataMu.RUnlock()
		return nil, fmt.Errorf("live image map not initialized")
	}

	val, ok := as.ClientRealtimeData[tag]
	if !ok {
		as.ClientRealtimeDataMu.RUnlock()
		return nil, fmt.Errorf("%w: live image not found for tag %d", types.LiveImageMissingError, tag)
	}

	// fmt.Printf("Retrieved live image for tag %d, size: %.2fMB\n", tag, float64(len(val.LiveImageBytes))/1024/1024)
	liveImage := val.LiveImageBytes
	if len(liveImage) == 0 {
		as.ClientRealtimeDataMu.RUnlock()
		// return nil, fmt.Errorf("live image bytes are nil for tag %d", tag)
		return nil, nil
	}
	if len(liveImage) == 0 || len(liveImage) > types.MaxLiveImageBytes {
		as.ClientRealtimeDataMu.RUnlock()
		return nil, fmt.Errorf("size of live image is out of range: %.2fMB", float64(len(liveImage))/1024/1024)
	}

	// Copy the bytes
	imageCopy := make([]byte, len(liveImage))
	copy(imageCopy, liveImage)
	// Release the lock after copying the live image bytes in case live image is being updated concurrently
	as.ClientRealtimeDataMu.RUnlock()
	return imageCopy, nil
	// as.ClientRealtimeDataMu.RUnlock()
	// return liveImage, nil
}

func UpdateLiveImageBytes(tag int64, imageBytes []byte) error {
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return types.CreateInvalidFieldError("tagnumber", err)
	}
	if len(imageBytes) <= 0 || len(imageBytes) > types.MaxLiveImageBytes {
		return fmt.Errorf("size of live image is out of range: %.2fMB", float64(len(imageBytes))/1024/1024)
	}
	as, err := GetAppState()
	if err != nil || as == nil {
		return fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	// Store a copy
	newImage := make([]byte, len(imageBytes))
	copy(newImage, imageBytes)
	as.ClientRealtimeDataMu.Lock()
	defer as.ClientRealtimeDataMu.Unlock()
	// fmt.Printf("Updating live image for tag %d, size: %.2fMB\n", tag, float64(len(newImage))/1024/1024)
	clientData := as.ClientRealtimeData[tag]
	clientData.Tagnumber = tag
	clientData.LiveImageBytes = newImage
	as.ClientRealtimeData[tag] = clientData
	return nil
}

// Retrieves the clientUUID field from the ClientRealtimeData map for the given tag
func GetRealtimeClientUUID(tag int64) (uuid.UUID, error) {
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return uuid.Nil, types.CreateInvalidFieldError("tagnumber", err)
	}

	as, err := GetAppState()
	if err != nil || as == nil {
		return uuid.Nil, fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	as.ClientRealtimeDataMu.RLock()
	defer as.ClientRealtimeDataMu.RUnlock()
	clientData, ok := as.ClientRealtimeData[tag]
	if !ok {
		return uuid.Nil, types.ErrClientNotFound
	}
	return clientData.ClientUUID, nil
}

// Updates the clientUUID field in the ClientRealtimeData map for the given tag
func SetRealtimeClientUUID(tag int64, uuid uuid.UUID) error {
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return types.CreateInvalidFieldError("tagnumber", err)
	}

	as, err := GetAppState()
	if err != nil || as == nil {
		return fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	as.ClientRealtimeDataMu.Lock()
	defer as.ClientRealtimeDataMu.Unlock()
	clientData := as.ClientRealtimeData[tag]
	clientData.Tagnumber = tag
	clientData.ClientUUID = uuid
	as.ClientRealtimeData[tag] = clientData
	return nil
}

func UpdateClientLastHeardInAppState(tag int64, lastHeard *time.Time) error {
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return types.CreateInvalidFieldError("tagnumber", err)
	}
	if lastHeard == nil || lastHeard.IsZero() {
		// return fmt.Errorf("lastHeard is nil")
		return nil // No need to update if lastHeard is nil or zero, just skip
	}

	appState, err := GetAppState()
	if err != nil || appState == nil {
		return fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	appState.ClientRealtimeDataMu.Lock()
	defer appState.ClientRealtimeDataMu.Unlock()
	clientData := appState.ClientRealtimeData[tag]
	clientData.Tagnumber = tag
	clientData.LastHeard = lastHeard
	clientData.LastHeardUpdatedInDB = false // Mark as not updated in DB, this function only gets used in post.go
	appState.ClientRealtimeData[tag] = clientData

	return nil
}

func ReplaceClientRealtimeData(tag int64, val *types.JobQueueRealtimeData) error {
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return types.CreateInvalidFieldError("tagnumber", err)
	}
	if val == nil {
		return fmt.Errorf("JobQueueRealtimeData struct is nil")
	}
	if val.LastHeard == nil || val.LastHeard.IsZero() {
		// return fmt.Errorf("lastHeard is nil, skipping update for tag %d", tag)
		return nil // No need to update if lastHeard is nil or zero, just skip
	}

	appState, err := GetAppState()
	if err != nil || appState == nil {
		return fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	valCopy := *val // Create a copy to avoid modifying the original struct
	appState.ClientRealtimeDataMu.Lock()
	defer appState.ClientRealtimeDataMu.Unlock()
	appState.ClientRealtimeData[tag] = valCopy

	return nil
}

func isClientOnline(clientData types.JobQueueRealtimeData) bool {
	if clientData.LastHeard == nil || clientData.LastHeard.IsZero() {
		return false
	}
	if clientData.LastHeard.Add(types.LastHeardTimeout).Before(time.Now()) {
		return false
	}
	return true
}

func GetAllOnlineClients() (onlineClientTags []int64, err error) {
	appState, err := GetAppState()
	if err != nil || appState == nil {
		return nil, fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	appState.ClientRealtimeDataMu.RLock()
	defer appState.ClientRealtimeDataMu.RUnlock()

	for tag := range appState.ClientRealtimeData {
		if clientData, ok := appState.ClientRealtimeData[tag]; ok {
			if isClientOnline(clientData) {
				onlineClientTags = append(onlineClientTags, tag)
			}
		}
	}
	return onlineClientTags, nil
}

func GetRealtimeClientData(tag int64) (*types.JobQueueRealtimeData, error) {
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return nil, types.CreateInvalidFieldError("tagnumber", err)
	}

	as, err := GetAppState()
	if err != nil || as == nil {
		return nil, fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	as.ClientRealtimeDataMu.RLock()
	defer as.ClientRealtimeDataMu.RUnlock()
	clientData, ok := as.ClientRealtimeData[tag]
	if !ok {
		return nil, fmt.Errorf("%w: live client data not found for tag %d", types.ErrClientNotFound, tag)
	}
	return &clientData, nil
}

func GetAllOnlineClientsData() (clientDataCopy map[int64]types.JobQueueRealtimeData, err error) {
	appState, err := GetAppState()
	if err != nil || appState == nil {
		return nil, fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}
	appState.ClientRealtimeDataMu.RLock()
	defer appState.ClientRealtimeDataMu.RUnlock()

	// Create a copy of the map to avoid race conditions
	clientDataCopy = make(map[int64]types.JobQueueRealtimeData, len(appState.ClientRealtimeData))
	for tag, clientData := range appState.ClientRealtimeData {
		if isClientOnline(clientData) {
			clientDataCopy[tag] = clientData
		}
	}

	return clientDataCopy, nil
}

func ClearOfflineLiveImageBytes() (entriesCleared int64, entriesSkipped int64, totalEntries int64) {
	as, err := GetAppState()
	if err != nil || as == nil {
		return entriesCleared, entriesSkipped, totalEntries
	}
	now := time.Now()
	as.ClientRealtimeDataMu.Lock()
	defer as.ClientRealtimeDataMu.Unlock()
	for tag, clientData := range as.ClientRealtimeData {
		if len(clientData.LiveImageBytes) > 0 {
			if clientData.LastHeard.IsZero() || clientData.LastHeard.Add(types.LastHeardTimeout).Before(now) {
				clientData.LiveImageBytes = nil
				as.ClientRealtimeData[tag] = clientData
				atomic.AddInt64(&entriesCleared, 1)
			} else {
				atomic.AddInt64(&totalEntries, 1)
			}
		} else {
			atomic.AddInt64(&entriesSkipped, 1)
			atomic.AddInt64(&totalEntries, 1)
		}
	}
	return entriesCleared, entriesSkipped, totalEntries
}

func GetAllClientRealtimeData() (clientDataCopy map[int64]types.JobQueueRealtimeData, err error) {
	appState, err := GetAppState()
	if err != nil || appState == nil {
		return nil, fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}
	appState.ClientRealtimeDataMu.RLock()
	defer appState.ClientRealtimeDataMu.RUnlock()

	// Create a copy of the map to avoid race conditions
	clientDataCopy = make(map[int64]types.JobQueueRealtimeData, len(appState.ClientRealtimeData))
	maps.Copy(clientDataCopy, appState.ClientRealtimeData)

	return clientDataCopy, nil
}

func UpdateClientAppUptime(tag int64, duration time.Duration) error {
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return types.CreateInvalidFieldError("tagnumber", err)
	}

	if duration < 0 {
		return fmt.Errorf("app uptime cannot be negative: %v", duration)
	}

	appState, err := GetAppState()
	if err != nil || appState == nil {
		return fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	appState.ClientRealtimeDataMu.Lock()
	defer appState.ClientRealtimeDataMu.Unlock()

	clientData := appState.ClientRealtimeData[tag]
	clientData.Tagnumber = tag
	clientData.AppUptime = duration
	appState.ClientRealtimeData[tag] = clientData

	return nil
}

func UpdateClientSystemUptime(tag int64, duration time.Duration) error {
	if err := types.IsTagnumberInt64Valid(tag); err != nil {
		return types.CreateInvalidFieldError("tagnumber", err)
	}

	if duration < 0 {
		return fmt.Errorf("system uptime cannot be negative: %v", duration)
	}

	appState, err := GetAppState()
	if err != nil || appState == nil {
		return fmt.Errorf("%w: %w", types.CannotGetAppStateError, err)
	}

	appState.ClientRealtimeDataMu.Lock()
	defer appState.ClientRealtimeDataMu.Unlock()

	clientData := appState.ClientRealtimeData[tag]
	clientData.Tagnumber = tag
	clientData.SystemUptime = duration
	appState.ClientRealtimeData[tag] = clientData

	return nil
}
