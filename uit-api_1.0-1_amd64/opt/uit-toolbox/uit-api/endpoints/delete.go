package endpoints

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"uit-api/database"
	"uit-api/types"
)

func DeleteImage(w http.ResponseWriter, req *http.Request) {
	log := GetLoggerFromContext(req.Context()).With(slog.String("func", "DeleteImage"))

	// Check if required query parameters are set
	if !req.URL.Query().Has("client_uuid") {
		log.Warnf("client_uuid query key not provided")
		WriteJsonError(w, http.StatusBadRequest)
		return
	}
	if !req.URL.Query().Has("file_uuid") {
		log.Warnf("file_uuid query key not provided")
		WriteJsonError(w, http.StatusBadRequest)
		return
	}

	clientUUID, err := GetUUIDFromQuery(req.URL.Query(), "client_uuid")
	if err != nil {
		log.Warnf("invalid client_uuid query parameter: %v", err)
		WriteJsonError(w, http.StatusBadRequest)
		return
	}
	fileUUID := GetStrQuery(req.URL.Query(), "file_uuid")
	if strings.TrimSpace(fileUUID) == "" {
		log.Warnf("invalid file_uuid query parameter")
		WriteJsonError(w, http.StatusBadRequest)
		return
	}

	// Get filepath from uuid
	imageManifest, err := database.GetClientImageManifestByFileUUID(req.Context(), fileUUID)
	if err != nil {
		log.Errorf("cannot retrieve image manifest: %v", err)
		WriteJsonError(w, http.StatusInternalServerError)
		return
	}
	// Check for a non-nil response from DB
	if imageManifest == nil {
		log.Warnf("image manifest not found for '%s'", fileUUID)
		WriteJsonError(w, http.StatusNotFound)
		return
	}

	// client UUID
	if imageManifest.ClientUUID == nil || strings.TrimSpace(*imageManifest.ClientUUID) == "" {
		log.Warnf("client UUID missing in image manifest for '%s'", fileUUID)
		WriteJsonError(w, http.StatusNotFound)
		return
	}
	clientUUIDFromManifest := strings.TrimSpace(*imageManifest.ClientUUID)
	if clientUUIDFromManifest != clientUUID.String() {
		log.Warnf("client UUID from manifest does not match client_uuid query parameter for '%s'", fileUUID)
		WriteJsonError(w, http.StatusBadRequest)
		return
	}

	// file uuid
	if imageManifest.FileUUID == nil || strings.TrimSpace(*imageManifest.FileUUID) == "" {
		log.Warnf("file not found in image manifest for '%s'", fileUUID)
		WriteJsonError(w, http.StatusNotFound)
		return
	}
	fileUUIDFromManifest := strings.TrimSpace(*imageManifest.FileUUID)
	if fileUUIDFromManifest != fileUUID {
		log.Warnf("file UUID from manifest does not match file_uuid query parameter for '%s'", fileUUID)
		WriteJsonError(w, http.StatusBadRequest)
		return
	}

	// file name
	if imageManifest.FileName == nil || strings.TrimSpace(*imageManifest.FileName) == "" {
		log.Warnf("no file name found in image manifest for provided file uuid '%s'", fileUUID)
		WriteJsonError(w, http.StatusNotFound)
		return
	}
	fileNameFromManifest := strings.TrimSpace(*imageManifest.FileName)
	if fileNameFromManifest == "" {
		log.Warnf("file name is empty in image manifest for '%s'", fileUUID)
		WriteJsonError(w, http.StatusNotFound)
		return
	}

	filePath := filepath.Join("/opt/inventory_images", clientUUIDFromManifest, fileNameFromManifest)
	filePath = filepath.Clean(filePath)

	imageFile, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warnf("image file found for '%s'", fileUUID)
			WriteJsonError(w, http.StatusNotFound)
			return
		}
		log.Errorf("unable to read image file: %v", err)
		WriteJsonError(w, http.StatusInternalServerError)
		return
	}
	if imageFile == nil {
		log.Warnf("image not found for provided uuid (%s) and file name (%s): ", fileUUID, filePath)
		WriteJsonError(w, http.StatusNotFound)
		return
	}
	defer imageFile.Close()

	fileMetadata, err := imageFile.Stat()
	if err != nil {
		log.Error("Error getting image file metadata: " + err.Error())
		WriteJsonError(w, http.StatusInternalServerError)
		return
	}
	if fileMetadata.IsDir() {
		log.Warn("Resolved image file path is a directory for provided uuid and file name: " + fileUUID)
		WriteJsonError(w, http.StatusNotFound)
		return
	}
	if fileMetadata.Size() == 0 {
		log.Warn("Resolved image file is empty for provided uuid and file name: " + fileUUID)
		// WriteJsonError(w, http.StatusNotFound)
		// return
	}
	if !fileMetadata.Mode().IsRegular() {
		log.Warn("Resolved image file is not a regular file for provided uuid and file name: " + fileUUID)
		WriteJsonError(w, http.StatusNotFound)
		return
	}

	if err := os.Remove(filePath); err != nil {
		log.Error("Error deleting image file: " + err.Error())
		WriteJsonError(w, http.StatusInternalServerError)
		return
	}

	if imageManifest.ThumbnailFileName != nil && strings.TrimSpace(*imageManifest.ThumbnailFileName) != "" {
		thumbnailPath := filepath.Join("/opt/inventory_images", clientUUIDFromManifest, *imageManifest.ThumbnailFileName)
		thumbnailPath = filepath.Clean(thumbnailPath)
		resolvedThumbnailPath, err := filepath.EvalSymlinks(thumbnailPath)
		if err != nil {
			log.Error("Error resolving thumbnail file path, continuing: " + err.Error())
			// WriteJsonError(w, http.StatusInternalServerError)
			// return
		}
		thumbnailFile, err := os.Open(resolvedThumbnailPath)
		if err != nil {
			log.Error("Error opening thumbnail file " + resolvedThumbnailPath + ", continuing: " + err.Error())
			// WriteJsonError(w, http.StatusInternalServerError)
			// return
		}
		if thumbnailFile != nil {
			thumbnailFile.Close()
		}
		if err := os.Remove(resolvedThumbnailPath); err != nil {
			log.Error("Error deleting thumbnail file " + resolvedThumbnailPath + ", continuing: " + err.Error())
			// WriteJsonError(w, http.StatusInternalServerError)
			// return
		}
	}

	if err := database.HideClientImageByUUID(req.Context(), fileUUID); err != nil {
		log.Error("DB error while deleting client image with UUID '" + fileUUID + "': " + err.Error())
		WriteJsonError(w, http.StatusInternalServerError)
		return
	}

	log.Infof("Successfully deleted client image with UUID '%s'", fileUUID)
	WriteJson(w, http.StatusOK, map[string]string{"message": "Image deleted successfully"})
}

func DeleteOSInfoByTagnumber(w http.ResponseWriter, req *http.Request) {
	log := GetLoggerFromContext(req.Context()).With(slog.String("func", "DeleteOSInfoByTagnumber"))

	querySerialVal := GetStrQuery(req.URL.Query(), "system_serial")
	if querySerialVal == "" {
		log.Warnf("No system_serial query key provided: %v", querySerialVal)
		WriteJsonError(w, http.StatusBadRequest)
		return
	}
	if err := types.IsSystemSerialValid(querySerialVal); err != nil {
		log.Warnf("Invalid system_serial query parameter: %v", err)
		WriteJsonError(w, http.StatusBadRequest)
		return
	}

	queryTagVal := GetInt64Query(req.URL.Query(), "tagnumber")
	if err := types.IsTagnumberInt64Valid(queryTagVal); err != nil {
		log.Warnf("Invalid tagnumber: %v", err)
		WriteJsonError(w, http.StatusBadRequest)
		return
	}

	if err := database.DeleteOSInfoByTagnumber(req.Context(), queryTagVal, querySerialVal); err != nil {
		log.Errorf("error deleting OS info for tagnumber '%d': %v", queryTagVal, err)
		WriteJsonError(w, http.StatusInternalServerError)
		return
	}

	log.Infof("successfully deleted OS info for '%d'", queryTagVal)
}
