package apisixregistryagent

import (
	"os"
	"testing"
)

func TestParseProtoHttpRules(t *testing.T) {
	protoContent := `
service TestService {
  rpc GetTest (TestRequest) returns (TestResponse) {
    option (google.api.http) = {
      get: "/v1/test/{id}"
    };
  }
  rpc PostTest (TestRequest) returns (TestResponse) {
    option (google.api.http) = {
      post: "/v1/test"
    };
  }
}
`
	tmpfile, err := os.CreateTemp("", "test-*.proto")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write([]byte(protoContent)); err != nil {
		t.Fatalf("failed to write proto: %v", err)
	}
	tmpfile.Close()

	routes, err := ParseProtoHttpRules(tmpfile.Name())
	if err != nil {
		t.Fatalf("ParseProtoHttpRules error: %v", err)
	}
	if len(routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes))
	}
	if routes[0]["uri"] != "/v1/test/{id}" || routes[1]["uri"] != "/v1/test" {
		t.Errorf("unexpected uri: %+v", routes)
	}
	if routes[0]["methods"].([]string)[0] != "get" || routes[1]["methods"].([]string)[0] != "post" {
		t.Errorf("unexpected methods: %+v", routes)
	}
}
