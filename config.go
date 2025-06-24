package apisixregistryagent

import (
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AdminAPI      string        `yaml:"admin_api"`
	AdminKey      string        `yaml:"admin_key"`
	ServiceName   string        `yaml:"service_name"`
	ServiceID     string        `yaml:"service_id"`
	ServicePort   int           `yaml:"service_port"`
	ProtoPath     string        `yaml:"proto_path"`
	RoutePlugins  []PluginSpec  `yaml:"route_plugins"`
	Upstream      *UpstreamSpec `yaml:"upstream,omitempty"`
	TTL           int           `yaml:"ttl"`
	MaxRetry      int           `yaml:"max_retry"`
	RetryInterval time.Duration `yaml:"retry_interval"`
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
			yaml.Unmarshal(data, cfg)
		}
	}
	// ENV 覆盖
	if v := os.Getenv("SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}
	if v := os.Getenv("SERVICE_ID"); v != "" {
		cfg.ServiceID = v
	}
	if v := os.Getenv("SERVICE_PORT"); v != "" {
		// parse int
	}
	if v := os.Getenv("PROTO_PATH"); v != "" {
		cfg.ProtoPath = v
	}
	return cfg, nil
}
