# apisix-registry-agent

A Go agent for automatic registration and deregistration of microservices to APISIX via the Admin API. Supports gRPC/REST, proto parsing, plugin templates, graceful shutdown, and is suitable for both local development and production environments.

## Features

- Auto-register Service, Route, Upstream, and Proto to APISIX on startup
- Parse `google.api.http` annotations in proto files to generate RESTful routes
- gRPC + grpc-transcode plugin support
- Plugin template support (e.g., Auth, Header rewrite)
- Graceful deregistration on shutdown (signal/TTL)
- Retry mechanism for registration failures (max_retry, interval, backoff)
- YAML config + ENV override for environment awareness
- CI/CD and registry integration ready

## Quick Start

1. **Add the module to your Go project** (local replace or go get)
2. **Configure `registry.yaml` or use environment variables**

```yaml
admin_api: "http://localhost:8000/apisix/admin"
admin_key: "your-admin-key"
service_name: "your-service"
service_id: "your-service"
service_port: 50051
proto_path: "../your-service/proto/your.proto"
route_plugins:
  - name: "grpc-transcode"
    config:
      proto_id: "your-service"
      service: "your.Service"
      deadline: 10
  - name: "key-auth"
    config: {}
ttl: 60
max_retry: 5
retry_interval: 2s
upstream:
  type: roundrobin
  nodes:
    "127.0.0.1:50051": 1
```

3. **Call the agent in your service startup:**

```go
import apisixagent "zenglow.io/apisix-registry-agent"

func main() {
    cfg, _ := apisixagent.LoadConfig("path/to/registry.yaml")
    go apisixagent.Run(cfg)
    // ...your service startup...
}
```

## Key Parameters

- `admin_api`: APISIX Admin API endpoint
- `admin_key`: APISIX Admin API KEY
- `service_name`/`service_id`: Logical service name/unique ID
- `service_port`: Local service port (used for upstream node generation)
- `proto_path`: Path to proto file (for proto/route auto-registration)
- `route_plugins`: Route plugin templates (e.g., grpc-transcode, auth)
- `ttl`: Registration TTL, supports auto-deregistration
- `max_retry`/`retry_interval`: Retry mechanism for registration
- `upstream`: Custom upstream config

## Deregistration & Graceful Shutdown

- Handles SIGINT/SIGTERM for auto-deregistration
- Supports TTL-based auto-deregistration

## Use Cases

- Microservice auto-registration to APISIX
- gRPC/RESTful API gateway exposure
- Local development, testing, and production automation

## Contributing & License

PRs and issues are welcome!

MIT License
