package apisixregistryagent

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
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

	upstream, err := BuildUpstream(opts)
	if err != nil {
		log.Printf("[APISIX-AGENT] BuildUpstream error: %v", err)
	} else {
		if err := client.RegisterUpstream(serviceID, upstream); err != nil {
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
	if err := client.RegisterService(serviceID, svc); err != nil {
		log.Printf("[APISIX-AGENT] RegisterService failed: %v", err)
		return err
	}
	log.Printf("[APISIX-AGENT] Service registered: %s", serviceID)
	// 3. 注册 Route
	routes, _ := ParseProtoHttpRules(cfg.ProtoPath)
	// log.Printf("--------------------------", routes)
	for i, r := range routes {
		if cfg.Debug {
			log.Printf("[APISIX-AGENT][DEBUG] parsed route: %+v", r) // 输出每个 route 解析结果
		}
		id := fmt.Sprintf("%s-%d", serviceID, i)
		route := map[string]interface{}{
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
		if err := client.RegisterRoute(id, route); err != nil {
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
				if err := client.RegisterProto(serviceID, content); err != nil {
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
	// log.Printf("[APISIX-AGENT] Waiting for shutdown signal...")
	// ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	// defer stop()
	// <-ctx.Done()
	// log.Printf("[APISIX-AGENT] Deregistering...")
	// client.DeleteService(serviceID)
	// for i := range routes {
	// 	id := fmt.Sprintf("%s-%d", serviceID, i)
	// 	client.DeleteRoute(id)
	// }
	// client.DeleteProto(serviceID)
	// if cfg.Upstream != nil {
	// 	client.DeleteUpstream(serviceID)
	// }
	// log.Printf("[APISIX-AGENT] Deregistration complete.")
	return nil
}

// encodeBase64 工具函数
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
