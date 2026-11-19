package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComposeTokenManagersReceiveAuthorizationRefreshPolicy(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatal(err)
	}
	// Decode YAML merges to check what each process actually inherits. The
	// Compose substitution must keep an explicit zero, with the runtime's
	// 72-hour default used only when the deployment setting is absent/empty.
	for _, service := range []string{"backend-api", "backend-worker"} {
		got := compose.Services[service].Environment["CAIYUN_AUTH_REFRESH_LEAD_HOURS"]
		if got != "${CAIYUN_AUTH_REFRESH_LEAD_HOURS:-72}" {
			t.Errorf("%s refresh policy = %q; explicit deployment values must reach TokenManager", service, got)
		}
	}
}

func TestComposeProxyTrustMatchesFrontendNetworkIdentity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
			Networks    interface{}       `yaml:"networks"`
		} `yaml:"services"`
		Networks map[string]struct {
			IPAM struct {
				Config []struct {
					Subnet  string `yaml:"subnet"`
					Gateway string `yaml:"gateway"`
					IPRange string `yaml:"ip_range"`
				} `yaml:"config"`
			} `yaml:"ipam"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatal(err)
	}
	frontend := compose.Services["frontend"]
	network := frontend.Networks.(map[string]interface{})["caiyun-network"].(map[string]interface{})
	address := network["ipv4_address"].(string)
	api := compose.Services["backend-api"].Environment
	if address != "${CAIYUN_FRONTEND_IP:-172.30.139.10}" || api["TRUSTED_PROXIES"] != "${TRUSTED_PROXIES:-"+address+"}" || api["REVERSE_PROXY_REQUIRED"] != "true" {
		t.Fatalf("proxy trust no longer matches the frontend identity: address=%s API=%v", address, api["TRUSTED_PROXIES"])
	}
	ipam := compose.Networks["caiyun-network"].IPAM.Config
	if len(ipam) != 1 || ipam[0].Subnet != "${CAIYUN_DOCKER_SUBNET:-172.30.139.0/24}" || ipam[0].Gateway != "${CAIYUN_DOCKER_GATEWAY:-172.30.139.1}" || ipam[0].IPRange != "${CAIYUN_DOCKER_DYNAMIC_RANGE:-172.30.139.128/25}" {
		t.Fatalf("frontend static address has no matching subnet/gateway or reserved dynamic range: %+v", ipam)
	}
	if frontend.Environment["NGINX_TRUSTED_PROXIES"] != "${TRUSTED_INGRESS_PROXIES:-${CAIYUN_DOCKER_GATEWAY:-172.30.139.1}}" {
		t.Fatal("Nginx no longer restores the trusted host proxy's client IP")
	}
}
