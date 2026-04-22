package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// @ai_doc 全局配置结构体，采用 YAML 文件加载 + 环境变量覆盖
// 核心逻辑：控制面和数据面共用同一份配置结构，通过 server.control_port / server.gateway_port 区分端口

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	JWT      JWTConfig      `yaml:"jwt"`
	Crypto   CryptoConfig   `yaml:"crypto"`
	Email    EmailConfig    `yaml:"email"`
	Log      LogConfig      `yaml:"log"`
	OAuth    OAuthConfig    `yaml:"oauth"`
	Gateway  GatewayConfig  `yaml:"gateway"`
}

// GatewayConfig 聚合数据面网关相关配置。
// Circuit 段控制账号级熔断（卡 #3a）。
type GatewayConfig struct {
	Upstream UpstreamConfig `yaml:"upstream"`
	Circuit  CircuitConfig  `yaml:"circuit"`
}

// CircuitConfig 账号级熔断参数。
//
// Enabled 的默认处理：如果整段都是零值（yaml 里没写），视为开启；
// 显式写了任意字段则尊重 yaml。与 UpstreamConfig 保持一致的启发式。
type CircuitConfig struct {
	Enabled                 bool    `yaml:"enabled"`
	ErrorRateThreshold      float64 `yaml:"error_rate_threshold"`
	MinRequestCount         int     `yaml:"min_request_count"`
	WindowSec               int     `yaml:"window_sec"`
	CooldownBaseMs          int     `yaml:"cooldown_base_ms"`
	CooldownMaxMs           int     `yaml:"cooldown_max_ms"`
	SuccessThresholdToClose int     `yaml:"success_threshold_to_close"`
	HalfOpenProbeRate       float64 `yaml:"half_open_probe_rate"`
	KeyTTLSec               int     `yaml:"key_ttl_sec"`
	// MaxAttempts ChatCompletions fallback 循环的最大尝试次数（含首次）。
	// 默认 3，即"原始 + 最多 2 次换账号重试"。<=0 会被 applyCircuitDefaults 纠正为 3。
	MaxAttempts int `yaml:"max_attempts"`
}

// UpstreamConfig 上游连接池与分项超时配置。
// 所有 *Sec 字段单位为秒，0 表示使用 httpclient 包的内部默认值。
type UpstreamConfig struct {
	Enabled                  bool `yaml:"enabled"`
	MaxIdleConns             int  `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost      int  `yaml:"max_idle_conns_per_host"`
	MaxConnsPerHost          int  `yaml:"max_conns_per_host"`
	IdleConnTimeoutSec       int  `yaml:"idle_conn_timeout_sec"`
	DialTimeoutSec           int  `yaml:"dial_timeout_sec"`
	TLSHandshakeTimeoutSec   int  `yaml:"tls_handshake_timeout_sec"`
	ResponseHeaderTimeoutSec int  `yaml:"response_header_timeout_sec"`
	StreamIdleTimeoutSec     int  `yaml:"stream_idle_timeout_sec"`
}

type OAuthConfig struct {
	PublicBaseURL string                        `yaml:"public_base_url"`
	Providers    map[string]OAuthProviderConfig `yaml:"providers"`
}

type OAuthProviderConfig struct {
	ClientID              string `yaml:"client_id"`
	ClientSecret          string `yaml:"client_secret"`
	AuthorizationEndpoint string `yaml:"authorization_endpoint"`
	TokenEndpoint         string `yaml:"token_endpoint"`
	Scopes                string `yaml:"scopes"`
}

type ServerConfig struct {
	ControlPort     int `yaml:"control_port"`
	AdminControlPort int `yaml:"admin_control_port"`
	GatewayPort     int `yaml:"gateway_port"`
	Mode            string `yaml:"mode"`
}

type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	Name            string `yaml:"name"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.Name)
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type JWTConfig struct {
	Secret     string `yaml:"secret"`
	AccessTTL  int    `yaml:"access_ttl"`
	RefreshTTL int    `yaml:"refresh_ttl"`
}

type CryptoConfig struct {
	AESKey string `yaml:"aes_key"`
}

type EmailConfig struct {
	SMTPHost string `yaml:"smtp_host"`
	SMTPPort int    `yaml:"smtp_port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
}

type LogConfig struct {
	Level    string `yaml:"level"`
	Format   string `yaml:"format"`
	Output   string `yaml:"output"`
	FilePath string `yaml:"file_path"`
}

var (
	global *Config
	once   sync.Once
)

func Load(path string) (*Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	if cfg.Server.AdminControlPort <= 0 {
		cfg.Server.AdminControlPort = 8083
	}
	applyUpstreamDefaults(&cfg.Gateway.Upstream)
	applyCircuitDefaults(&cfg.Gateway.Circuit)
	overrideFromEnv(&cfg)
	once.Do(func() { global = &cfg })
	return &cfg, nil
}

func Get() *Config {
	return global
}

// applyUpstreamDefaults 给 Upstream 配置填默认值。
//
// Enabled 特殊处理：Go 的 bool 零值是 false，无法区分"yaml 里没写"和"yaml 里显式写了 false"。
// 本函数采用启发式：如果 Upstream 段所有数值字段都为 0，视为 yaml 里根本没写该段，
// 此时把 Enabled 默认设为 true；只要用户显式写了任一字段（包括 enabled: false），
// 就尊重 yaml 的配置，不再覆盖 Enabled。
func applyUpstreamDefaults(u *UpstreamConfig) {
	allZero := u.MaxIdleConns == 0 && u.MaxIdleConnsPerHost == 0 && u.MaxConnsPerHost == 0 &&
		u.IdleConnTimeoutSec == 0 && u.DialTimeoutSec == 0 && u.TLSHandshakeTimeoutSec == 0 &&
		u.ResponseHeaderTimeoutSec == 0 && u.StreamIdleTimeoutSec == 0 && !u.Enabled
	if allZero {
		u.Enabled = true
	}
	if u.MaxIdleConns <= 0 {
		u.MaxIdleConns = 256
	}
	if u.MaxIdleConnsPerHost <= 0 {
		u.MaxIdleConnsPerHost = 64
	}
	if u.MaxConnsPerHost <= 0 {
		u.MaxConnsPerHost = 128
	}
	if u.IdleConnTimeoutSec <= 0 {
		u.IdleConnTimeoutSec = 90
	}
	if u.DialTimeoutSec <= 0 {
		u.DialTimeoutSec = 5
	}
	if u.TLSHandshakeTimeoutSec <= 0 {
		u.TLSHandshakeTimeoutSec = 10
	}
	if u.ResponseHeaderTimeoutSec <= 0 {
		u.ResponseHeaderTimeoutSec = 30
	}
	if u.StreamIdleTimeoutSec <= 0 {
		u.StreamIdleTimeoutSec = 60
	}
}

// applyCircuitDefaults 与 applyUpstreamDefaults 同款启发式：
// 整段零值视为未配置 → Enabled=true；显式写任一字段就尊重 yaml。
func applyCircuitDefaults(c *CircuitConfig) {
	allZero := c.ErrorRateThreshold == 0 && c.MinRequestCount == 0 && c.WindowSec == 0 &&
		c.CooldownBaseMs == 0 && c.CooldownMaxMs == 0 && c.SuccessThresholdToClose == 0 &&
		c.HalfOpenProbeRate == 0 && c.KeyTTLSec == 0 && c.MaxAttempts == 0 && !c.Enabled
	if allZero {
		c.Enabled = true
	}
	if c.ErrorRateThreshold <= 0 {
		c.ErrorRateThreshold = 0.5
	}
	if c.MinRequestCount <= 0 {
		c.MinRequestCount = 20
	}
	if c.WindowSec <= 0 {
		c.WindowSec = 60
	}
	if c.CooldownBaseMs <= 0 {
		c.CooldownBaseMs = 5000
	}
	if c.CooldownMaxMs <= 0 {
		c.CooldownMaxMs = 60000
	}
	if c.SuccessThresholdToClose <= 0 {
		c.SuccessThresholdToClose = 3
	}
	if c.HalfOpenProbeRate <= 0 {
		c.HalfOpenProbeRate = 0.3
	}
	if c.KeyTTLSec <= 0 {
		c.KeyTTLSec = 600
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
}

func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil && p > 0 {
			cfg.Database.Port = p
		}
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.Database.Name = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWT.Secret = v
	}
	if v := os.Getenv("AES_KEY"); v != "" {
		cfg.Crypto.AESKey = v
	}
	if v := os.Getenv("ADMIN_CONTROL_PORT"); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil && p > 0 {
			cfg.Server.AdminControlPort = p
		}
	}
	if v := os.Getenv("TRAFFIC_UPSTREAM_ENABLED"); v != "" {
		switch v {
		case "true", "1", "TRUE", "True", "yes", "YES":
			cfg.Gateway.Upstream.Enabled = true
		case "false", "0", "FALSE", "False", "no", "NO":
			cfg.Gateway.Upstream.Enabled = false
		}
	}
	if v := os.Getenv("TRAFFIC_CIRCUIT_ENABLED"); v != "" {
		switch v {
		case "true", "1", "TRUE", "True", "yes", "YES":
			cfg.Gateway.Circuit.Enabled = true
		case "false", "0", "FALSE", "False", "no", "NO":
			cfg.Gateway.Circuit.Enabled = false
		}
	}
}
