package types

import (
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

type AppState struct {
	AppStateMu           sync.Mutex
	SessionSecret        []byte
	ClientRealtimeDataMu sync.RWMutex
	ClientRealtimeData   map[int64]JobQueueRealtimeData
}

type AppConfiguration struct {
	FormConstraints      atomic.Pointer[HTMLFormConstraints]
	FileConstraints      atomic.Pointer[FileUploadConstraints]
	LogLevel             string         `json:"UIT_SERVER_LOG_LEVEL"`
	AdminPasswd          string         `json:"UIT_SERVER_ADMIN_PASSWD"`
	DBName               string         `json:"UIT_SERVER_DB_NAME"`
	ServerHostname       string         `json:"UIT_SERVER_HOSTNAME"`
	WANAddr              netip.Addr     `json:"UIT_SERVER_WAN_IP_ADDRESS"`
	LANAddr              netip.Addr     `json:"UIT_SERVER_LAN_IP_ADDRESS"`
	WANIfaceName         string         `json:"UIT_SERVER_WAN_IF"`
	LANIfaceName         string         `json:"UIT_SERVER_LAN_IF"`
	AllowedWANIPs        []netip.Prefix `json:"UIT_SERVER_WAN_ALLOWED_IP"`
	AllowedLANIPs        []netip.Prefix `json:"UIT_SERVER_LAN_ALLOWED_IP"`
	AllAllowedIPs        []netip.Prefix `json:"UIT_SERVER_ANY_ALLOWED_IP"`
	AllowedWANMap        sync.Map       // map[string]netip.Prefix
	AllowedLANMap        sync.Map       // map[string]netip.Prefix
	AllAllowedMap        sync.Map       // map[string]netip.Prefix
	WebUserDefaultPasswd string         `json:"UIT_WEB_USER_DEFAULT_PASSWD"`
	WebDBUsername        string         `json:"UIT_WEB_DB_USERNAME"`
	WebDBPasswd          string         `json:"UIT_WEB_DB_PASSWD"`
	WebDBName            string         `json:"UIT_WEB_DB_NAME"`
	WebDBHost            netip.Addr     `json:"UIT_WEB_DB_HOST"`
	WebDBPort            uint16         `json:"UIT_WEB_DB_PORT"`
	WebHTTPAddr          netip.Addr     `json:"UIT_WEB_HTTP_HOST"`
	WebHTTPPort          uint16         `json:"UIT_WEB_HTTP_PORT"`
	WebHTTPSAddr         netip.Addr     `json:"UIT_WEB_HTTPS_HOST"`
	WebHTTPSPort         uint16         `json:"UIT_WEB_HTTPS_PORT"`
	WebTLSCertFile       string         `json:"UIT_WEB_TLS_CERT_FILE"`
	WebTLSKeyFile        string         `json:"UIT_WEB_TLS_KEY_FILE"`
	APIRequestTimeout    time.Duration  `json:"UIT_WEB_API_REQUEST_TIMEOUT"`
	FileRequestTimeout   time.Duration  `json:"UIT_WEB_FILE_REQUEST_TIMEOUT"`
	RateLimitBurst       int            `json:"UIT_WEB_RATE_LIMIT_BURST"`
	RateLimitInterval    float64        `json:"UIT_WEB_RATE_LIMIT_INTERVAL"`
	RateLimitTimeout     time.Duration  `json:"UIT_WEB_RATE_LIMIT_BAN_DURATION"`
	ClientDBUser         string         `json:"UIT_CLIENT_DB_USER"`
	ClientDBPasswd       string         `json:"UIT_CLIENT_DB_PASSWD"`
	ClientDBName         string         `json:"UIT_CLIENT_DB_NAME"`
	ClientDBHost         netip.Addr     `json:"UIT_CLIENT_DB_HOST"`
	ClientDBPort         uint16         `json:"UIT_CLIENT_DB_PORT"`
	ClientNTPHost        netip.Addr     `json:"UIT_CLIENT_NTP_HOST"`
	ClientPingHost       netip.Addr     `json:"UIT_CLIENT_PING_HOST"`
	WebmasterName        string         `json:"UIT_WEBMASTER_NAME"`
	WebmasterEmail       string         `json:"UIT_WEBMASTER_EMAIL"`
	WebEndpoints         sync.Map       // map[string]WebEndpointConfig
}

type ClientConfig struct {
	UIT_CLIENT_DB_USER   string `json:"UIT_CLIENT_DB_USER"`
	UIT_CLIENT_DB_PASSWD string `json:"UIT_CLIENT_DB_PASSWD"`
	UIT_CLIENT_DB_NAME   string `json:"UIT_CLIENT_DB_NAME"`
	UIT_CLIENT_DB_HOST   string `json:"UIT_CLIENT_DB_HOST"`
	UIT_CLIENT_DB_PORT   string `json:"UIT_CLIENT_DB_PORT"`
	UIT_CLIENT_NTP_HOST  string `json:"UIT_CLIENT_NTP_HOST"`
	UIT_CLIENT_PING_HOST string `json:"UIT_CLIENT_PING_HOST"`
	UIT_SERVER_HOSTNAME  string `json:"UIT_SERVER_HOSTNAME"`
	UIT_WEB_HTTP_HOST    string `json:"UIT_WEB_HTTP_HOST"`
	UIT_WEB_HTTP_PORT    string `json:"UIT_WEB_HTTP_PORT"`
	UIT_WEB_HTTPS_HOST   string `json:"UIT_WEB_HTTPS_HOST"`
	UIT_WEB_HTTPS_PORT   string `json:"UIT_WEB_HTTPS_PORT"`
	UIT_WEBMASTER_NAME   string `json:"UIT_WEBMASTER_NAME"`
	UIT_WEBMASTER_EMAIL  string `json:"UIT_WEBMASTER_EMAIL"`
}

type WebEndpointConfig struct {
	FilePath        string   `json:"file_path"`
	AllowedMethods  []string `json:"allowed_methods"`
	TLSRequired     *bool    `json:"tls_required"`
	AuthRequired    *bool    `json:"auth_required"`
	Requires        []string `json:"requires"`
	ACLUsers        []string `json:"acl_users"`
	ACLGroups       []string `json:"acl_groups"`
	HTTPVersion     string   `json:"http_version"`
	EndpointType    string   `json:"endpoint_type"`
	ContentType     string   `json:"content_type"`
	StatusCode      int      `json:"status_code"`
	Redirect        *bool    `json:"redirect"`
	RedirectURL     string   `json:"redirect_url"`
	MaxUploadSize   *int64   `json:"max_upload_size"`
	MaxDownloadSize *int64   `json:"max_download_size"`
}
