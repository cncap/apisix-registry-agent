package apisixregistryagent

import (
	"io"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// APISIX 管理 API 地址和密钥
	// 支持通过环境变量 APISIX_ADMIN_API 和 APISIX_ADMIN_KEY 设置
	// 如果未设置，则使用默认值 http://
	AdminAPI       string        `yaml:"admin_api"`
	AdminKey       string        `yaml:"admin_key"`
	ServiceVersion string        `yaml:"service_version"`
	ServiceName    string        `yaml:"service_name"`
	ServiceID      string        `yaml:"service_id"`
	ServicePort    int           `yaml:"service_port"`
	ProtoPath      string        `yaml:"proto_path"`
	ProtoPbPath    string        `yaml:"proto_pb_path"`
	RoutePlugins   []PluginSpec  `yaml:"route_plugins"`
	Upstream       *UpstreamSpec `yaml:"upstream,omitempty"`
	TTL            int           `yaml:"ttl"`
	MaxRetry       int           `yaml:"max_retry"`
	RetryInterval  time.Duration `yaml:"retry_interval"`
}

type PluginSpec struct {
	Name   string                 `yaml:"name"`
	Config map[string]interface{} `yaml:"config"`
}

type UpstreamSpec struct {
	Type  string         `yaml:"type"`
	Nodes map[string]int `yaml:"nodes"`
}

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		AdminAPI: os.Getenv("APISIX_ADMIN_API"),
		AdminKey: os.Getenv("APISIX_ADMIN_KEY"),
	}
	if file, err := os.Open(path); err == nil {
		defer file.Close()
		if data, err := io.ReadAll(file); err == nil {
			// 支持 ENV 占位符自动替换
			content := os.ExpandEnv(string(data))
			yaml.Unmarshal([]byte(content), cfg)
		}
	}
	// ENV 覆盖
	if v := os.Getenv("APISIX_ADMIN_API"); v != "" {
		cfg.AdminAPI = v
	}
	if v := os.Getenv("APISIX_ADMIN_KEY"); v != "" {
		cfg.AdminKey = v
	}
	if v := os.Getenv("SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}
	if v := os.Getenv("SERVICE_ID"); v != "" {
		cfg.ServiceID = v
	}
	if v := os.Getenv("SERVICE_GRPC_PORT"); v != "" {
		// parse int
	}
	if v := os.Getenv("PROTO_PATH"); v != "" {
		cfg.ProtoPath = v
	}
	if v := os.Getenv("PROTO_PB_PATH"); v != "" {
		cfg.ProtoPbPath = v
	}
	if v := os.Getenv("REGISTRY_TTL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.TTL = n
		}
	}
	if v := os.Getenv("REGISTRY_MAX_RETRY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxRetry = n
		}
	}
	if v := os.Getenv("REGISTRY_RETRY_INTERVAL"); v != "" {
		if duration, err := time.ParseDuration(v); err == nil {
			cfg.RetryInterval = duration
		}
	}
	if cfg.TTL < 60 {
		cfg.TTL = 60 // 默认值
	}
	if cfg.MaxRetry <= 0 {
		cfg.MaxRetry = 3 // 默认值
	}
	if cfg.RetryInterval < 3*time.Second {
		cfg.RetryInterval = 3 * time.Second
	}
	return cfg, nil
}
