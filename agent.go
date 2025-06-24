package apisixregistryagent

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// Agent 启动自动注册/反注册流程
func Run(cfg *Config) error {
	client := NewApisixClient(cfg)
	serviceID := cfg.ServiceID
	if serviceID == "" {
		serviceID = cfg.ServiceName
	}
	// 1. 注册 Service
	svc := map[string]interface{}{
		"id":       serviceID,
		"name":     cfg.ServiceName,
		"upstream": map[string]interface{}{"type": "roundrobin", "nodes": map[string]int{fmt.Sprintf("127.0.0.1:%d", cfg.ServicePort): 1}},
	}
	if cfg.Upstream != nil {
		svc["upstream"] = cfg.Upstream
	}
	if err := client.RegisterService(serviceID, svc); err != nil {
		return err
	}
	// 2. 注册 Route
	routes, _ := ParseProtoHttpRules(cfg.ProtoPath)
	for i, r := range routes {
		id := fmt.Sprintf("%s-%d", serviceID, i)
		r["service_id"] = serviceID
		if len(cfg.RoutePlugins) > 0 {
			plugins := map[string]interface{}{}
			for _, p := range cfg.RoutePlugins {
				plugins[p.Name] = p.Config
			}
			r["plugins"] = plugins
		}
		client.RegisterRoute(id, r)
	}
	// 3. 注册 Proto
	if cfg.ProtoPath != "" {
		if file, err := os.Open(cfg.ProtoPath); err == nil {
			defer file.Close()
			if protoContent, err := io.ReadAll(file); err == nil {
				client.RegisterProto(serviceID, string(protoContent))
			}
		}
	}
	// 4. 注册 Upstream（可选）
	if cfg.Upstream != nil {
		client.RegisterUpstream(serviceID, map[string]interface{}{
			"type":  cfg.Upstream.Type,
			"nodes": cfg.Upstream.Nodes,
		})
	}
	// 5. 捕获退出信号，自动反注册
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	client.DeleteService(serviceID)
	for i := range routes {
		id := fmt.Sprintf("%s-%d", serviceID, i)
		client.DeleteRoute(id)
	}
	client.DeleteProto(serviceID)
	if cfg.Upstream != nil {
		client.DeleteUpstream(serviceID)
	}
	return nil
}
