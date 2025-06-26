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

		if err != nil {
			return nil, err
		}
		req.Header.Set("X-API-KEY", c.AdminKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := http.DefaultClient.Do(req)
		respBody, _ := io.ReadAll(resp.Body)

		if c.Debug {
			log.Printf("[APISIX-AGENT][DEBUG] %s %s request body: %s \n", method, url, string(data))
		}
		// log.Printf("[APISIX-AGENT][DEBUG] %s %s request body: %s \n", method, url, string(data))

		if resp != nil {
			defer resp.Body.Close()
		}
		if err == nil && resp.StatusCode < 300 {
			return respBody, nil
		} else {
			log.Printf("[APISIX-AGENT][WARN] %s %s failed: status=%d, err=%v, resp=%s", method, url, resp.StatusCode, err, string(respBody))
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
	_, err := c.doRequest("POST", "/protos", body)
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
