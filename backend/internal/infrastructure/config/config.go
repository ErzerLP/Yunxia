package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 表示后端运行时配置。
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Storage  StorageConfig
	WebDAV   WebDAVConfig
	Aria2    Aria2Config
	Security SecurityConfig
	Logging  LoggingConfig
}

// ServerConfig 表示 HTTP 服务配置。
type ServerConfig struct {
	Host string
	Port int
	Mode string
}

// DatabaseConfig 表示数据库配置。
type DatabaseConfig struct {
	DSN string
}

// JWTConfig 表示令牌配置。
type JWTConfig struct {
	Secret             string
	AccessTokenExpire  time.Duration
	RefreshTokenExpire time.Duration
}

// StorageConfig 表示存储与上传相关配置。
type StorageConfig struct {
	DataDir          string
	TempDir          string
	DefaultChunkSize int64
	MaxUploadSize    int64
}

// WebDAVConfig 表示 WebDAV 配置。
type WebDAVConfig struct {
	Enabled bool
	Prefix  string
}

// Aria2Config 表示 Aria2 集成配置。
type Aria2Config struct {
	RPCURL      string
	RPCSecret   string
	DownloadDir string
}

// SecurityConfig 表示安全配置。
type SecurityConfig struct {
	BcryptCost        int
	LoginMaxAttempts  int
	LoginLockDuration time.Duration
}

// LoggingConfig 表示运行日志配置。
type LoggingConfig struct {
	Level            string
	Format           string
	AddSource        bool
	AccessLogEnabled bool
}

// Load 读取默认值并应用环境变量覆盖。
func Load() (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("YUNXIA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	accessTTL, err := time.ParseDuration(v.GetString("jwt.access_token_expire"))
	if err != nil {
		return Config{}, err
	}
	refreshTTL, err := time.ParseDuration(v.GetString("jwt.refresh_token_expire"))
	if err != nil {
		return Config{}, err
	}
	lockDuration, err := time.ParseDuration(v.GetString("security.login_lock_duration"))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Server: ServerConfig{
			Host: v.GetString("server.host"),
			Port: v.GetInt("server.port"),
			Mode: v.GetString("server.mode"),
		},
		Database: DatabaseConfig{
			DSN: v.GetString("database.dsn"),
		},
		JWT: JWTConfig{
			Secret:             v.GetString("jwt.secret"),
			AccessTokenExpire:  accessTTL,
			RefreshTokenExpire: refreshTTL,
		},
		Storage: StorageConfig{
			DataDir:          v.GetString("storage.data_dir"),
			TempDir:          v.GetString("storage.temp_dir"),
			DefaultChunkSize: v.GetInt64("storage.default_chunk_size"),
			MaxUploadSize:    v.GetInt64("storage.max_upload_size"),
		},
		WebDAV: WebDAVConfig{
			Enabled: v.GetBool("webdav.enabled"),
			Prefix:  v.GetString("webdav.prefix"),
		},
		Aria2: Aria2Config{
			RPCURL:      v.GetString("aria2.rpc_url"),
			RPCSecret:   v.GetString("aria2.rpc_secret"),
			DownloadDir: v.GetString("aria2.download_dir"),
		},
		Security: SecurityConfig{
			BcryptCost:        v.GetInt("security.bcrypt_cost"),
			LoginMaxAttempts:  v.GetInt("security.login_max_attempts"),
			LoginLockDuration: lockDuration,
		},
		Logging: LoggingConfig{
			Level:            v.GetString("logging.level"),
			Format:           v.GetString("logging.format"),
			AddSource:        v.GetBool("logging.add_source"),
			AccessLogEnabled: v.GetBool("logging.access_log_enabled"),
		},
	}

	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("database.dsn", "./data/database.db")
	v.SetDefault("jwt.secret", "change-me-in-production")
	v.SetDefault("jwt.access_token_expire", "15m")
	v.SetDefault("jwt.refresh_token_expire", "168h")
	v.SetDefault("storage.data_dir", "./data/storage")
	v.SetDefault("storage.temp_dir", "./data/temp")
	v.SetDefault("storage.default_chunk_size", 5*1024*1024)
	v.SetDefault("storage.max_upload_size", int64(10*1024*1024*1024))
	v.SetDefault("webdav.enabled", true)
	v.SetDefault("webdav.prefix", "/dav")
	v.SetDefault("aria2.rpc_url", "http://aria2:6800/jsonrpc")
	v.SetDefault("aria2.rpc_secret", "")
	v.SetDefault("aria2.download_dir", "")
	v.SetDefault("security.bcrypt_cost", 12)
	v.SetDefault("security.login_max_attempts", 5)
	v.SetDefault("security.login_lock_duration", "15m")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.add_source", false)
	v.SetDefault("logging.access_log_enabled", true)
}
