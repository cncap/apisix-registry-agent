package apisixregistryagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type ApisixClient struct {
	Debug         bool
	AdminAPI      string
	AdminKey      string
	MaxRetry      int
	RetryInterval time.Duration
}

func NewApisixClient(cfg *Config) *ApisixClient {
	return &ApisixClient{
		Debug:         cfg.Debug,
		AdminAPI:      cfg.AdminAPI,
		AdminKey:      cfg.AdminKey,
		MaxRetry:      cfg.MaxRetry,
		RetryInterval: cfg.RetryInterval,
	}
}

func (c *ApisixClient) doRequest(method, path string, body interface{}) ([]byte, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s%s", c.AdminAPI, path)
	for i := 0; i < c.MaxRetry; i++ {
		req, err := http.NewRequest(method, url, bytes.NewReader(data))

		if c.Debug {
			log.Printf("[APISIX-AGENT][DEBUG] %s %s request body: %s \n", method, url, string(data))
		}

		if err != nil {
			return nil, err
		}

		req.Header.Set("X-API-KEY", c.AdminKey)

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := http.DefaultClient.Do(req)
		var respBody []byte

		if resp != nil {
			defer resp.Body.Close()
			respBody, _ = io.ReadAll(resp.Body)
		}

		if c.Debug {
			log.Printf("[APISIX-AGENT][DEBUG] %s %s response body: %s \n", method, url, string(respBody))
		}

		if err == nil && resp != nil && resp.StatusCode < 300 {
			return respBody, nil
		} else {
			statusCode := 0
			if resp != nil {
				statusCode = resp.StatusCode
			}
			log.Printf("[APISIX-AGENT][WARN] %s %s failed: status=%d, err=%v, resp=%s", method, url, statusCode, err, string(respBody))
		}
		time.Sleep(c.RetryInterval * time.Duration(i+1))
	}
	return nil, fmt.Errorf("APISIX request failed after %d retries", c.MaxRetry)
}

// Service/Route/Upstream/Proto 注册、反注册接口
func (c *ApisixClient) RegisterService(id string, svc map[string]interface{}) error {
	_, err := c.doRequest("PUT", "/services/"+id, svc)
	return err
}
func (c *ApisixClient) DeleteService(id string) error {
	_, err := c.doRequest("DELETE", "/services/"+id, nil)
	return err
}
func (c *ApisixClient) RegisterRoute(id string, route map[string]interface{}) error {
	_, err := c.doRequest("PUT", "/routes/"+id, route)
	return err
}
func (c *ApisixClient) DeleteRoute(id string) error {
	_, err := c.doRequest("DELETE", "/routes/"+id, nil)
	return err
}
func (c *ApisixClient) RegisterProto(id string, protoContent string) error {
	body := map[string]interface{}{"content": protoContent}
	_, err := c.doRequest("PUT", "/protos/"+id, body)
	return err
}
func (c *ApisixClient) DeleteProto(id string) error {
	_, err := c.doRequest("DELETE", "/protos/"+id, nil)
	return err
}
func (c *ApisixClient) RegisterUpstream(id string, upstream map[string]interface{}) error {
	_, err := c.doRequest("PUT", "/upstreams/"+id, upstream)
	return err
}
func (c *ApisixClient) DeleteUpstream(id string) error {
	_, err := c.doRequest("DELETE", "/upstreams/"+id, nil)
	return err
}

// RegisterConsumers 自动注册 APISIX Consumer，支持 multi-auth
func RegisterConsumers(client *ApisixClient, consumers []ConsumerConfig) {
	for _, c := range consumers {
		plugins := map[string]interface{}{}
		if c.JwtEnabled {
			plugins["jwt-auth"] = map[string]interface{}{"key": c.Name}
		}
		if c.KeyAuthEnabled && c.KeyAuthKey != "" {
			plugins["key-auth"] = map[string]interface{}{"key": c.KeyAuthKey}
		}
		if len(plugins) == 0 {
			log.Printf("[APISIX-AGENT] Consumer %s: no auth plugin enabled, skip", c.Name)
			continue
		}
		consumer := map[string]interface{}{
			"username": c.Name,
			"plugins":  plugins,
		}
		path := "/consumers/" + c.Name
		// 幂等注册，已存在则跳过
		_, err := client.doRequest("PUT", path, consumer)
		if err != nil {
			log.Printf("[APISIX-AGENT] RegisterConsumer failed for %s: %v", c.Name, err)
		} else {
			log.Printf("[APISIX-AGENT] Consumer registered: %s", c.Name)
		}
	}
}
