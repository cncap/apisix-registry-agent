package apisixregistryagent

import (
	"io"
	"log"
	"os"
	"regexp"
	"strings"
)

// 解析 proto 文件中的 google.api.http 注解，生成 APISIX 路由规则
func ParseProtoHttpRules(protoPath string) ([]map[string]interface{}, error) {
	file, err := os.Open(protoPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	content := string(data)
	// 只用 package 字段，不用 go_package

	// 更宽松的 serviceRe，允许 service 结尾的 } 前有任意空格
	serviceRe := regexp.MustCompile(`service\s+([a-zA-Z0-9_]+)\s*\{([\s\S]*)\}`)
	// 最宽松的 rpcRe，匹配所有 rpc ... { ... } 块
	// rpcRe := regexp.MustCompile(`(?m)^rpc\s+([a-zA-Z0-9_]+)\s*\([^\)]*\)\s*returns\s*\([^\)]*\)\s*\{([\s\S]*?)^\}`)
	// debug: 输出 service 匹配结果
	services := serviceRe.FindAllStringSubmatch(content, -1)
	if len(services) == 0 {
		log.Printf("[PROTO-PARSER] No service matched! Check proto format.")
	} else {
		log.Printf("[PROTO-PARSER] Matched %d service(s)", len(services))
	}
	var routes []map[string]interface{}
	for _, svc := range services {
		serviceName := svc[1]
		serviceBody := svc[2]
		log.Printf("[PROTO-PARSER] serviceBody for %s (len=%d): ...%s", serviceName, len(serviceBody), serviceBody[len(serviceBody)-100:])
		// 用 strings.Split 分割每个 rpc
		rpcBlocks := strings.Split(serviceBody, "rpc ")
		if len(rpcBlocks) <= 1 {
			log.Printf("[PROTO-PARSER] No rpc matched in service %s!", serviceName)
			continue
		}
		for _, block := range rpcBlocks[1:] { // 第一个是空串
			// 提取方法名
			nameRe := regexp.MustCompile(`^([a-zA-Z0-9_]+)\s*\([^)]+\)\s*returns\s*\([^)]+\)\s*\{`)
			nameMatch := nameRe.FindStringSubmatch(block)
			if nameMatch == nil {
				continue
			}
			methodName := nameMatch[1]
			// 提取 option 块（到第一个 }）
			endIdx := strings.Index(block, "}")
			if endIdx == -1 {
				continue
			}
			options := block[:endIdx+1]
			// 查找 http 注解
			httpRe := regexp.MustCompile(`option \(google.api.http\) = \{[\s\S]*?(get|post|put|delete): "([^"]+)"[\s\S]*?\}`)
			if m := httpRe.FindStringSubmatch(options); len(m) >= 3 {
				httpMethod := m[1]
				uri := m[2]
				methodUpper := strings.ToUpper(httpMethod)
				grpcMethod := methodName
				route := map[string]interface{}{
					"uri":         uri,
					"method":      methodUpper,
					"methods":     []string{methodUpper},
					"grpc_method": grpcMethod,
				}
				log.Printf("[PROTO-PARSER] Parsed route: %+v", route)
				routes = append(routes, route)
			}
		}
	}
	return routes, nil
}
