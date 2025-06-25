package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	apisixagent "github.com/cncap/apisix-registry-agent"
)

func main() {
	var (
		configPath              = flag.String("config", "./registry.yaml", "Path to registry.yaml config file")
		env                     = flag.String("env", "dev", "Environment: dev or prod")
		useDiscovery            = flag.Bool("use-discovery", false, "Enable service discovery for upstream")
		discoveryType           = flag.String("discovery-type", "", "Discovery type: dns, kubernetes, ...")
		staticNode              = flag.String("static-node", "", "Static node for upstream, e.g. host:port=1")
		serviceNameForDiscovery = flag.String("discovery-service-name", "", "Service name for discovery")
	)
	flag.Parse()

	os.Setenv("REGISTRY_ENV", *env)
	os.Setenv("REGISTRY_USE_DISCOVERY", func() string {
		if *useDiscovery {
			return "true"
		} else {
			return "false"
		}
	}())
	os.Setenv("REGISTRY_DISCOVERY_TYPE", *discoveryType)
	os.Setenv("REGISTRY_DISCOVERY_SERVICE_NAME", *serviceNameForDiscovery)

	staticNodes := make(map[string]int)
	if *staticNode != "" {
		var hostport string
		var weight int
		_, err := fmt.Sscanf(*staticNode, "%[^=]=%d", &hostport, &weight)
		if err == nil {
			staticNodes[hostport] = weight
		}
	}
	cfg, err := apisixagent.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("[APISIX-AGENT] Failed to load config: %v", err)
	}
	if len(staticNodes) > 0 {
		if cfg.Upstream == nil {
			cfg.Upstream = &apisixagent.UpstreamSpec{}
		}
		cfg.Upstream.Nodes = staticNodes
	}
	apisixagent.Run(cfg)
}
