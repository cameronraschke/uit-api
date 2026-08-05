package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync/atomic"
	"time"
	"uit-api/types"
)

var (
	appConfigInstance atomic.Pointer[types.AppConfiguration]
)

func InitAppConfig() error {
	var appConfigCopy types.AppConfiguration

	// Decode config file JSON
	mainConfigFile, err := os.ReadFile("/etc/uit-toolbox/uit-toolbox.json")
	if err != nil {
		return fmt.Errorf("failed to read config '/etc/uit-toolbox/uit-toolbox.json': %w", err)
	}
	if err := json.Unmarshal(mainConfigFile, &appConfigCopy); err != nil {
		return fmt.Errorf("failed to unmarshal config JSON: %w", err)
	}

	// Convert durations to seconds
	appConfigCopy.APIRequestTimeout *= time.Second
	appConfigCopy.FileRequestTimeout *= time.Second
	appConfigCopy.RateLimitTimeout *= time.Second

	// WAN interface, IP, and allowed IPs
	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to get network interfaces: %w", err)
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return fmt.Errorf("failed to get addresses for WAN interface: %w", err)
		}
		wanIPFound := false
		lanIPFound := false
		for _, addr := range addrs {
			convIP, ok := addr.(*net.IPNet)
			if !ok {
				return fmt.Errorf("address is not an IPNet: %v", addr)
			}
			if iface.Name == appConfigCopy.WANIfaceName && convIP.IP.String() == appConfigCopy.WANAddr.String() {
				wanIPFound = true
			}
			if iface.Name == appConfigCopy.LANIfaceName && convIP.IP.String() == appConfigCopy.LANAddr.String() {
				lanIPFound = true
			}
		}
		if iface.Name == appConfigCopy.WANIfaceName && !wanIPFound {
			return fmt.Errorf("WAN interface %s does not have the expected IP address %s", appConfigCopy.WANIfaceName, appConfigCopy.WANAddr.String())
		}
		if iface.Name == appConfigCopy.LANIfaceName && !lanIPFound {
			return fmt.Errorf("LAN interface %s does not have the expected IP address %s", appConfigCopy.LANIfaceName, appConfigCopy.LANAddr.String())
		}
	}

	// Populate allowed IPs
	for _, wanIP := range appConfigCopy.AllowedWANIPs {
		wanIPCopy := wanIP
		appConfigCopy.AllowedWANMap.Store(&wanIPCopy, true)
		appConfigCopy.AllAllowedMap.Store(&wanIPCopy, true)
	}

	for _, lanIP := range appConfigCopy.AllowedLANIPs {
		lanIPCopy := lanIP
		appConfigCopy.AllowedLANMap.Store(&lanIPCopy, true)
		appConfigCopy.AllAllowedMap.Store(&lanIPCopy, true)
	}

	// Set input constraints
	generalNoteConstraints := &types.GeneralNoteConstraints{
		MaxFormBytes: 32768 * 4, // Reasonable Unicode/JSON overhead
	}

	inventoryFormConstraints := &types.InventoryUpdateFormConstraints{
		MaxJSONBytes:                 1 << 20,
		AcquiredDateIsMandatory:      false,
		RetiredDateIsMandatory:       false,
		IsFunctionalIsMandatory:      false,
		DiskRemovedIsMandatory:       false,
		LastHardwareCheckIsMandatory: false,
		CheckoutBoolIsMandatory:      false,
		CheckoutDateIsMandatory:      false,
		ReturnDateIsMandatory:        false,
	}

	// Set form constraints
	formConstraints := &types.HTMLFormConstraints{
		GeneralNote:   generalNoteConstraints,
		InventoryForm: inventoryFormConstraints,
	}
	appConfigCopy.FormConstraints.Store(formConstraints)

	// Set file upload constraints
	imgConstraints := types.ImageUploadConstraints{
		MinFileSize:  512,
		MaxFileSize:  20 << 20,
		MaxFileCount: 20,
		AcceptedImageExtensionsAndMimeTypes: map[string]string{
			".jpg":  "image/jpeg",
			".jpeg": "image/jpeg",
			".png":  "image/png",
			".jfif": "image/jpeg",
		},
	}
	vidConstraints := types.VideoUploadConstraints{
		MinFileSize:  512,
		MaxFileSize:  100 << 20,
		MaxFileCount: 5,
		AcceptedVideoExtensionsAndMimeTypes: map[string]string{
			".mp4": "video/mp4",
		},
	}
	fileConstraints := &types.FileUploadConstraints{
		ImageConstraints:       &imgConstraints,
		VideoConstraints:       &vidConstraints,
		MaxUploadFileSizeLimit: 512 << 20,
	}
	appConfigCopy.FileConstraints.Store(fileConstraints)

	appConfigInstance.Store(&appConfigCopy)
	return nil
}

func GetAppConfig() (*types.AppConfiguration, error) {
	ac := appConfigInstance.Load()
	if ac == nil {
		return nil, fmt.Errorf("%w", types.NilAppConfigError)
	}
	return ac, nil
}

// Webserver config
func GetWebServerIPs() (httpIP string, httpsIP string, err error) {
	ac, err := GetAppConfig()
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", types.NilAppConfigError, err)
	}
	return ac.WebHTTPAddr.String(), ac.WebHTTPSAddr.String(), nil
}

func GetServerIPAddressByInterface(ifName string) (string, error) {
	if ifName == "" {
		return "", errors.New("interface name is empty")
	}
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return "", fmt.Errorf("failed to get interface %s: %w", ifName, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get addresses for interface %s: %w", ifName, err)
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("no valid IP address found for interface %s", ifName)
}

func GetWebmasterContact() (webmasterName string, webmasterEmail string, err error) {
	ac, err := GetAppConfig()
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", types.NilAppConfigError, err)
	}
	return ac.WebmasterName, ac.WebmasterEmail, nil
}

func GetClientConfig() (*types.ClientConfig, error) {
	ac, err := GetAppConfig()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", types.NilAppConfigError, err)
	}
	appConfigCopy := ac

	clientConfig := &types.ClientConfig{
		UIT_CLIENT_DB_USER:   appConfigCopy.ClientDBUser,
		UIT_CLIENT_DB_PASSWD: appConfigCopy.ClientDBPasswd,
		UIT_CLIENT_DB_NAME:   appConfigCopy.ClientDBName,
		UIT_CLIENT_DB_HOST:   appConfigCopy.ClientDBHost.String(),
		UIT_CLIENT_DB_PORT:   strconv.FormatUint(uint64(appConfigCopy.ClientDBPort), 10),
		UIT_CLIENT_NTP_HOST:  appConfigCopy.ClientNTPHost.String(),
		UIT_CLIENT_PING_HOST: appConfigCopy.ClientPingHost.String(),
		UIT_SERVER_HOSTNAME:  appConfigCopy.ServerHostname,
		UIT_WEB_HTTP_HOST:    appConfigCopy.WebHTTPAddr.String(),
		UIT_WEB_HTTP_PORT:    strconv.FormatUint(uint64(appConfigCopy.WebHTTPPort), 10),
		UIT_WEB_HTTPS_HOST:   appConfigCopy.WebHTTPSAddr.String(),
		UIT_WEB_HTTPS_PORT:   strconv.FormatUint(uint64(appConfigCopy.WebHTTPSPort), 10),
		UIT_WEBMASTER_NAME:   appConfigCopy.WebmasterName,
		UIT_WEBMASTER_EMAIL:  appConfigCopy.WebmasterEmail,
	}
	return clientConfig, nil
}

func GetTLSCertFiles() (certFile string, keyFile string, err error) {
	ac, err := GetAppConfig()
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", types.NilAppConfigError, err)
	}
	return ac.WebTLSCertFile, ac.WebTLSKeyFile, nil
}

func GetAllowedLANIPs() ([]netip.Prefix, error) {
	ac, err := GetAppConfig()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", types.NilAppConfigError, err)
	}
	var allowedIPs []netip.Prefix
	ac.AllowedLANMap.Range(func(k, v any) bool {
		ipRangePtr, ok := k.(*netip.Prefix)
		if !ok || ipRangePtr == nil {
			return true
		}
		ipRange := *ipRangePtr
		if ipRange == (netip.Prefix{}) {
			return true
		}
		allowedIPs = append(allowedIPs, ipRange)
		return true
	})
	return allowedIPs, nil
}
