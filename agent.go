package apisixregistryagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type Options struct {
	Env                     string         // "dev" or "prod"
	UseDiscovery            bool           // true = use dynamic service discovery
	DiscoveryType           string         // "dns", "kubernetes", ...
	StaticNodes             map[string]int // only used if UseDiscovery == false
	ServiceNameForDiscovery string         // e.g. "zenglow-auth-service.default.svc.cluster.local"
	ServiceID               string         // for GenerateServiceName
	Port                    int            // for GenerateServiceName
}

func GenerateServiceName(opts Options) string {
	if opts.Env == "dev" {
		return fmt.Sprintf("%s:%d", opts.ServiceID, opts.Port)
	}
	if opts.DiscoveryType == "kubernetes" {
		return fmt.Sprintf("%s.default.svc.cluster.local", opts.ServiceID)
	}
	return opts.ServiceID
}

func BuildUpstream(opts Options) (map[string]interface{}, error) {
	upstream := map[string]interface{}{
		"id":     opts.ServiceID,
		"type":   "roundrobin",
		"scheme": "grpc",
	}
	if opts.UseDiscovery {
		discoveryType := opts.DiscoveryType
		if discoveryType == "" {
			discoveryType = "dns"
		}
		upstream["discovery_type"] = discoveryType
		if opts.ServiceNameForDiscovery != "" {
			upstream["service_name"] = opts.ServiceNameForDiscovery
		} else {
			upstream["service_name"] = GenerateServiceName(opts)
		}
	} else if len(opts.StaticNodes) > 0 {
		upstream["nodes"] = opts.StaticNodes
	} else {
		return nil, fmt.Errorf("no upstream nodes or discovery config provided")
	}
	return upstream, nil
}

// Agent 启动自动注册/反注册流程
func Run(cfg *Config) error {
	client := NewApisixClient(cfg)
	serviceID := cfg.ServiceID
	if serviceID == "" {
		serviceID = cfg.ServiceName
	}
	log.Printf("[APISIX-AGENT] Registering service: %s", serviceID)
	// 1. 注册 Upstream（支持服务发现/静态节点）
	opts := Options{
		Env:                     os.Getenv("REGISTRY_ENV"),
		UseDiscovery:            os.Getenv("REGISTRY_USE_DISCOVERY") == "true",
		DiscoveryType:           os.Getenv("REGISTRY_DISCOVERY_TYPE"),
		ServiceNameForDiscovery: os.Getenv("REGISTRY_DISCOVERY_SERVICE_NAME"),
		ServiceID:               serviceID,
		Port:                    cfg.ServicePort,
		StaticNodes:             map[string]int{fmt.Sprintf("127.0.0.1:%d", cfg.ServicePort): 1},
	}
	if cfg.Upstream != nil && len(cfg.Upstream.Nodes) > 0 {
		opts.StaticNodes = cfg.Upstream.Nodes
	}

	// 1.5 自动注册 APISIX Consumer（multi-auth）
	if len(cfg.Consumers) > 0 {
		RegisterConsumers(client, cfg.Consumers)
	}

	upstream, err := BuildUpstream(opts)
	if err != nil {
		log.Printf("[APISIX-AGENT] BuildUpstream error: %v", err)
	} else {
		if err := registerUpstreamWithRetry(client, serviceID, upstream); err != nil {
			log.Printf("[APISIX-AGENT] RegisterUpstream failed: %v", err)
		} else {
			log.Printf("[APISIX-AGENT] Upstream registered: %s", serviceID)
		}
	}
	// 2. 注册 Service
	svc := map[string]interface{}{
		"id":          serviceID,
		"name":        cfg.ServiceName,
		"desc":        "Auto registered by apisix-registry-agent",
		"upstream_id": serviceID,
	}
	if err := registerServiceWithRetry(client, serviceID, svc); err != nil {
		log.Printf("[APISIX-AGENT] RegisterService failed: %v", err)
		return err
	}
	log.Printf("[APISIX-AGENT] Service registered: %s", serviceID)
	// 3. 注册 Route
	var customRouteMap map[string]interface{}
	if len(cfg.Routes) > 0 {
		customRouteMap = make(map[string]interface{})
		for _, cr := range cfg.Routes {
			customRouteMap[cr.URI] = cr
		}
	}
	routes, _ := ParseProtoHttpRules(cfg.ProtoPath)
	for i, r := range routes {
		id := fmt.Sprintf("%s-%d", serviceID, i)
		var route map[string]interface{}
		// 优先使用自定义路由配置
		if customRouteMap != nil {
			if cr, ok := customRouteMap[r["uri"].(string)]; ok {
				// 用自定义配置覆盖 proto 解析结果
				route = map[string]interface{}{
					"id":         id,
					"name":       id,
					"desc":       "Auto registered by apisix-registry-agent (custom config)",
					"service_id": serviceID,
					"uri":        cr.(RouteConfig).URI,
				}
				if len(cr.(RouteConfig).Methods) > 0 {
					route["methods"] = cr.(RouteConfig).Methods
				}
				if len(cr.(RouteConfig).Plugins) > 0 {
					plugins := map[string]interface{}{}
					for _, p := range cr.(RouteConfig).Plugins {
						pluginConfig := make(map[string]interface{})
						for k, v := range p.Config {
							pluginConfig[k] = v
						}
						// 自动补全 grpc-transcode method 字段
						if p.Name == "grpc-transcode" {
							if pluginConfig["method"] == nil {
								if gm, ok := r["grpc_method"]; ok && gm != nil && gm != "" {
									pluginConfig["method"] = gm
									if cfg.Debug {
										log.Printf("[APISIX-AGENT][DEBUG] auto fill grpc-transcode method: %v", gm)
									}
								} else {
									log.Printf("[APISIX-AGENT] ERROR: grpc-transcode method missing for custom route %v, please set method field", cr.(RouteConfig).URI)
								}
							}
							// 自动补全 proto_id 字段
							if pluginConfig["proto_id"] == nil && cfg.ProtoPath != "" {
								pluginConfig["proto_id"] = serviceID
								if cfg.Debug {
									log.Printf("[APISIX-AGENT][DEBUG] auto fill grpc-transcode proto_id: %v", serviceID)
								}
							}
							// 自动补全 service 字段
							if pluginConfig["service"] == nil && cfg.ServiceName != "" {
								pluginConfig["service"] = "micro." + capitalize(cfg.ServiceName) + "Service"
								if cfg.Debug {
									log.Printf("[APISIX-AGENT][DEBUG] auto fill grpc-transcode service: %v", pluginConfig["service"])
								}
							}
							// 强校验必填字段
							for _, key := range []string{"method", "proto_id", "service"} {
								if pluginConfig[key] == nil || pluginConfig[key] == "" {
									log.Printf("[APISIX-AGENT] ERROR: grpc-transcode %s missing for custom route %v, please set %s field", key, cr.(RouteConfig).URI, key)
								}
							}
						}
						plugins[p.Name] = pluginConfig
					}
					route["plugins"] = plugins
				}
				if cfg.Debug {
					log.Printf("[APISIX-AGENT][DEBUG] custom route to register: %+v", route)
				}
				if err := client.RegisterRoute(id, route); err != nil {
					log.Printf("[APISIX-AGENT] RegisterRoute failed: %v", err)
				} else {
					log.Printf("[APISIX-AGENT] Route registered: %s %v", id, route)
				}
				continue
			}
		}
		// ...原有 proto 解析注册逻辑...
		if cfg.Debug {
			log.Printf("[APISIX-AGENT][DEBUG] parsed route: %+v", r)
		}
		route = map[string]interface{}{
			"id":         id,
			"name":       id,
			"desc":       "Auto registered by apisix-registry-agent",
			"service_id": serviceID,
			"uri":        r["uri"],
		}
		if ms, ok := r["methods"]; ok {
			route["methods"] = ms
		}
		if len(cfg.RoutePlugins) > 0 {
			plugins := map[string]interface{}{}
			for _, p := range cfg.RoutePlugins {
				pluginConfig := make(map[string]interface{})
				for k, v := range p.Config {
					pluginConfig[k] = v
				}
				if p.Name == "grpc-transcode" {
					gm, ok := r["grpc_method"]
					if !ok || gm == nil || gm == "" {
						log.Printf("[APISIX-AGENT] ERROR: grpc_method not found for route %v, skip grpc-transcode method", r)
						continue
					}
					pluginConfig["method"] = gm
					if cfg.Debug {
						log.Printf("[APISIX-AGENT][DEBUG] grpc-transcode method set: %v", gm)
					}
				}
				plugins[p.Name] = pluginConfig
			}
			route["plugins"] = plugins
		}
		if cfg.Debug {
			log.Printf("[APISIX-AGENT][DEBUG] final route to register: %+v", route)
		}
		if err := registerRouteWithRetry(client, id, route); err != nil {
			log.Printf("[APISIX-AGENT] RegisterRoute failed: %v", err)
		} else {
			log.Printf("[APISIX-AGENT] Route registered: %s %v", id, route)
		}
	}
	// 4. 注册 Proto
	if cfg.ProtoPbPath != "" {
		// 判断是否为 .pb 文件（descriptor），需要 base64 编码
		if file, err := os.Open(cfg.ProtoPbPath); err == nil {
			defer file.Close()
			if protoContent, err := io.ReadAll(file); err == nil {
				var content string
				if len(cfg.ProtoPbPath) > 3 && cfg.ProtoPbPath[len(cfg.ProtoPbPath)-3:] == ".pb" {
					// .pb 文件，base64 编码
					content = encodeBase64(protoContent)
				} else {
					// 普通 proto 文件，直接用文本
					content = string(protoContent)
				}
				if err := registerProtoWithRetry(client, serviceID, content); err != nil {
					log.Printf("[APISIX-AGENT] RegisterProto failed: %v", err)
				} else {
					log.Printf("[APISIX-AGENT] Proto registered: %s", serviceID)
				}
			} else {
				log.Printf("[APISIX-AGENT] Error reading proto file: %v", err)
			}
		} else {
			log.Printf("[APISIX-AGENT] Error opening proto file: %v", err)
		}
	}
	// 5. 捕获退出信号，自动反注册
	log.Printf("[APISIX-AGENT] Waiting for shutdown signal...")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Printf("[APISIX-AGENT] Deregistering...")
	// 彻底清理所有与 proto_id 相关的路由
	var failedRoutes []string
	protoRoutes, _ := ParseProtoHttpRules(cfg.ProtoPath)
	for i := range protoRoutes {
		id := fmt.Sprintf("%s-%d", serviceID, i)
		if err := deleteRouteWithRetry(client, id); err != nil {
			log.Printf("[APISIX-AGENT][Warn] Delete route error . %v", err)
			failedRoutes = append(failedRoutes, id)
		}
	}
	// 检查 APISIX 是否还有残留路由引用 proto_id
	if len(failedRoutes) > 0 {
		log.Printf("[APISIX-AGENT][Warn] Some routes failed to delete: %v", failedRoutes)
	}
	// 主动查询 APISIX 路由，彻底清理所有 proto_id 相关路由
	if err := forceDeleteProtoRelatedRoutes(client, serviceID); err != nil {
		log.Printf("[APISIX-AGENT][Warn] forceDeleteProtoRelatedRoutes: %v", err)
	}
	if err := deleteProtoWithRetry(client, serviceID); err != nil {
		log.Printf("[APISIX-AGENT][Warn] DeleteProto error: %v", err)
	}
	deleteServiceWithRetry(client, serviceID)
	if cfg.Upstream != nil {
		deleteUpstreamWithRetry(client, serviceID)
	}
	log.Printf("[APISIX-AGENT] Deregistration complete.")
	return nil
}

// encodeBase64 工具函数
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// capitalize 首字母大写
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// 幂等重试工具
func retryN(n int, op func() error, desc string) error {
	var err error
	for i := 0; i < n; i++ {
		err = op()
		if err == nil {
			return nil
		}
		log.Printf("[APISIX-AGENT][Retry] %s failed (try %d/%d): %v", desc, i+1, n, err)
	}
	return err
}

// RegisterRoute 幂等重试
func registerRouteWithRetry(client *ApisixClient, id string, route map[string]interface{}) error {
	desc := "RegisterRoute " + id
	return retryN(3, func() error {
		return client.RegisterRoute(id, route)
	}, desc)
}

// DeleteRoute 幂等重试
func deleteRouteWithRetry(client *ApisixClient, id string) error {
	desc := "DeleteRoute " + id
	return retryN(3, func() error {
		return client.DeleteRoute(id)
	}, desc)
}

// RegisterService 幂等重试
func registerServiceWithRetry(client *ApisixClient, id string, svc map[string]interface{}) error {
	desc := "RegisterService " + id
	return retryN(3, func() error {
		return client.RegisterService(id, svc)
	}, desc)
}

// RegisterUpstream 幂等重试
func registerUpstreamWithRetry(client *ApisixClient, id string, upstream map[string]interface{}) error {
	desc := "RegisterUpstream " + id
	return retryN(3, func() error {
		return client.RegisterUpstream(id, upstream)
	}, desc)
}

// RegisterProto 幂等重试
func registerProtoWithRetry(client *ApisixClient, id string, content string) error {
	desc := "RegisterProto " + id
	return retryN(3, func() error {
		return client.RegisterProto(id, content)
	}, desc)
}

// DeleteProto 幂等重试
func deleteProtoWithRetry(client *ApisixClient, id string) error {
	desc := "DeleteProto " + id
	return retryN(3, func() error {
		return client.DeleteProto(id)
	}, desc)
}

// DeleteService 幂等重试
func deleteServiceWithRetry(client *ApisixClient, id string) error {
	desc := "DeleteService " + id
	return retryN(3, func() error {
		return client.DeleteService(id)
	}, desc)
}

// DeleteUpstream 幂等重试
func deleteUpstreamWithRetry(client *ApisixClient, id string) error {
	desc := "DeleteUpstream " + id
	return retryN(3, func() error {
		return client.DeleteUpstream(id)
	}, desc)
}

// 强制彻底清理所有引用 proto_id 的路由
func forceDeleteProtoRelatedRoutes(client *ApisixClient, protoID string) error {
	// 查询所有路由
	resp, err := client.doRequest("GET", "/routes", nil)
	if err != nil {
		return fmt.Errorf("query routes failed: %w", err)
	}
	var data struct {
		Nodes []struct {
			Value map[string]interface{} `json:"value"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(resp, &data); err != nil {
		return fmt.Errorf("unmarshal routes failed: %w", err)
	}
	for _, node := range data.Nodes {
		v := node.Value
		plugins, ok := v["plugins"].(map[string]interface{})
		if !ok {
			continue
		}
		gt, ok := plugins["grpc-transcode"].(map[string]interface{})
		if !ok {
			continue
		}
		if pid, ok := gt["proto_id"].(string); ok && pid == protoID {
			if id, ok := v["id"].(string); ok {
				log.Printf("[APISIX-AGENT][Warn] Force delete route %s referencing proto_id %s", id, protoID)
				client.DeleteRoute(id)
			}
		}
	}
	return nil
}
