package apisixregistryagent

import (
	"io/ioutil"
	"regexp"
)

// 解析 proto 文件中的 google.api.http 注解，生成 APISIX 路由规则
func ParseProtoHttpRules(protoPath string) ([]map[string]interface{}, error) {
	data, err := ioutil.ReadFile(protoPath)
	if err != nil {
		return nil, err
	}
	content := string(data)
	// 简单正则提取 http 注解（仅演示，建议用 protoc plugin 生产环境更健壮）
	re := regexp.MustCompile(`option \(google.api.http\) = \{\s*(get|post|put|delete): \"([^"]+)\"`)
	matches := re.FindAllStringSubmatch(content, -1)
	var routes []map[string]interface{}
	for _, m := range matches {
		method := m[1]
		uri := m[2]
		routes = append(routes, map[string]interface{}{
			"uri":     uri,
			"methods": []string{method},
		})
	}
	return routes, nil
}
