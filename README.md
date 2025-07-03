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
- **[NEW]Automatic Consumer Registration** (multi-auth/JWT/key-auth, auto key field completion)
- **[NEW]Automatic sync of anonymous HTTP/gRPC routes**: All routes without multi-auth/jwt-auth/key-auth plugins are auto-synced to Go service anonymous whitelist (authctx.SetAnonymousPaths/SetAnonymousGRPCPaths), with hot-reload
- **[NEW]Idempotent and robust registration/deregistration**: All operations (Service, Upstream, Route, Proto) are retried and safe for repeated calls
- **[NEW]Strong validation and auto-completion**: grpc-transcode plugin required fields (method/proto_id/service) are auto-filled and strictly checked

## Quick Start

1. **Add the module to your Go project** (local replace or go get)
2. **Configure `registry-config.yaml` or use environment variables**

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
import apisixagent "github.com/cncap/apisix-registry-agent"

func main() {
    cfg, _ := apisixagent.LoadConfig("path/to/registry-config.yaml")
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

## Upstream Strategy

The apisix-registry-agent supports both static node and service discovery upstream registration, making it easy to switch between local development and production environments with no code changes.

### Static Node Upstream (dev)
- In development (`env=dev`), the agent registers upstreams with static IP/port nodes.
- Example:
  ```yaml
  upstream:
    type: roundrobin
    nodes:
      "127.0.0.1:50051": 1
  ```
- Or via CLI:
  ```sh
  registry-agent --env dev --static-node auth:8082=1
  ```

### Service Discovery Upstream (prod)
- In production (`env=prod`), the agent can register upstreams using service discovery (e.g., Kubernetes, DNS).
- Example:
  ```yaml
  upstream:
    type: roundrobin
    discovery_type: kubernetes
    service_name: auth.default.svc.cluster.local
    scheme: grpc
  ```
- Or via CLI:
  ```sh
  registry-agent --env prod --use-discovery true --discovery-type kubernetes --discovery-service-name auth.default.svc.cluster.local
  ```

### How it works
- The agent inspects the environment and configuration to decide which upstream strategy to use.
- If `UseDiscovery` is true, it registers a discovery-based upstream; otherwise, it uses static nodes.
- All logic is controlled by config, environment variables, or CLI flags—no code changes required for environment switching.

### Example: Switching Environments
- **Local/dev:**
  ```sh
  registry-agent --env dev --static-node 127.0.0.1:50051=1
  ```
- **Production/Kubernetes:**
  ```sh
  registry-agent --env prod --use-discovery true --discovery-type kubernetes --discovery-service-name auth.default.svc.cluster.local
  ```

### Test Cases
- Dev: registers static upstream with nodes
- Prod: registers discovery-based upstream with service_name/discovery_type

---

This strategy enables seamless migration from local to cloud-native environments with zero code modification—just change your config or CLI flags!

## CLI Usage

You can run the agent with flexible CLI flags to override config and environment:

```sh
registry-agent --config ./registry-config.yaml \
  --env dev \
  --static-node 127.0.0.1:50051=1

registry-agent --env prod \
  --use-discovery true \
  --discovery-type kubernetes \
  --discovery-service-name auth.default.svc.cluster.local
```

- `--env`: Switch between dev and prod
- `--use-discovery`: Enable service discovery for upstream
- `--discovery-type`: Set discovery type (e.g., kubernetes, dns)
- `--static-node`: Add static node (host:port=weight), can be used multiple times
- `--discovery-service-name`: Set service name for discovery

## Test Coverage

- `TestBuildUpstream_Static`: Validates static node upstream registration
- `TestBuildUpstream_Discovery`: Validates discovery-based upstream registration
- Add more tests for CLI parsing and config merging as needed

## Contributing & License

PRs and issues are welcome!

MIT License

## CHANGELOG

### v1.0.2 (2025-07-03)
- Feature: Automatic sync of all routes without multi-auth/jwt-auth/key-auth plugins to Go service anonymous whitelist (authctx.SetAnonymousPaths/SetAnonymousGRPCPaths), including gRPC method support, with hot-reload.
- Feature: Automatic APISIX Consumer Registration: Supports multi-auth (JWT/key-auth) consumer registration based on config, with auto key field completion for jwt-auth.
- Feature: Route Registration with Strong Validation: Custom route config takes precedence; auto-completes grpc-transcode required fields (method/proto_id/service) and validates presence.
- Feature: Idempotent and Robust Deregistration: Deregistration only loops protoRoutes, ensuring all proto_id-related routes are deleted. Proactively queries APISIX for any remaining routes referencing proto_id and force deletes them.
- Feature: Automatic Anonymous Path Sync: On agent startup (and hot-reload), all routes in registry-config.yaml without multi-auth/jwt-auth/key-auth plugins are automatically synced to Go service's anonymous whitelist (authctx.SetAnonymousPaths/SetAnonymousGRPCPaths), supporting both HTTP and gRPC method whitelists.
- Enhancement: All registration/deregistration operations are now idempotent and retried for robustness.
- Enhancement: Deregistration logic unified to only loop protoRoutes, ensuring no route is missed.
- Enhancement: Strong validation and auto-completion for grpc-transcode plugin fields.
- Enhancement: Config reload now uses hash check to avoid unnecessary reloads.
- Docs: Updated feature list and changelog to reflect new automation and robustness features.

### v1.0.1 (2025-06-25)
- Feature: Upstream registration now supports both static node and service discovery (Kubernetes/DNS) strategies.
- Feature: CLI flags for `--env`, `--use-discovery`, `--discovery-type`, `--static-node`, `--discovery-service-name`.
- Feature: Environment variable and CLI override for all major options.
- Feature: Helper functions for service name generation and upstream construction.
- Enhancement: Improved logging and error output for registration failures.
- Enhancement: Test coverage for both static and discovery upstream registration.
- Docs: Added Upstream Strategy, CLI Usage, and Test Coverage sections.
- Breaking: Service/Upstream registration logic now fully compatible with APISIX v3 Admin API.

### v1.0.0 (2024-06-01)
- Initial release: automatic APISIX registration for service, route, upstream, proto.
- Supports proto parsing, plugin templates, graceful shutdown, retry, YAML+ENV config.
- Compatible with APISIX v2 Admin API.
