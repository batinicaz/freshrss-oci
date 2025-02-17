package test

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"golang.org/x/exp/maps"
)

var (
	ingressTypes = map[string]string{
		"80":   "http",
		"1337": "http",
		"3000": "http",
		"8080": "http",
		"8081": "http",
	}
)

func runTerraform(t *testing.T, imageID string, providerConf ociProviderConfig) {
	terraformOptions := &terraform.Options{
		TerraformDir: "./terraform",
		Vars: map[string]interface{}{
			"image_id":               imageID,
			"ingress_ports":          maps.Keys(ingressTypes),
			"oci_fingerprint":        providerConf.fingerprint,
			"oci_private_key_path":   providerConf.privateKeyFilePath,
			"oci_region":             providerConf.region,
			"oci_tenancy_id":         providerConf.rootTenancyOCID,
			"oci_user_id":            providerConf.userOCID,
			"subnet_ocid":            subnetOCID,
			"terraform_tenancy_ocid": providerConf.terraformTenancyOCID,
		},
	}
	terraform.Init(t, terraformOptions)
	if !skipDestroy {
		defer terraform.Destroy(t, terraformOptions)
	}
	terraform.Apply(t, terraformOptions)

	publicIP := terraform.Output(t, terraformOptions, "public_ip")

	for port, protocol := range ingressTypes {
		if port == "8081" {
			// While redlib having issues being blocked by reddit accept 404's
			validateHTTPConnectionWithRetry(t, protocol, publicIP, port, http.StatusOK, http.StatusNotFound)
			continue
		}
		validateHTTPConnectionWithRetry(t, protocol, publicIP, port)
	}
}

func validateHTTPConnectionWithRetry(t *testing.T, protocol string, instanceIP string, port string, statusCodes ...int) {
	if len(statusCodes) == 0 {
		statusCodes = append(statusCodes, http.StatusOK)
	}
	t.Logf("Testing the service running on port %s", port)
	t.Logf("Will accept status codes: %v", statusCodes)
	maxRetries := 10
	sleepBetweenRetries := 10 * time.Second
	retry.DoWithRetry(t, "HTTP Request", maxRetries, sleepBetweenRetries, func() (string, error) {
		url := fmt.Sprintf("%s://%s:%s", protocol, instanceIP, port)
		req, err := http.NewRequest("GET", url, nil)

		if err != nil {
			return "", err
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}

		defer resp.Body.Close()
		if !slices.Contains(statusCodes, resp.StatusCode) {
			return "", fmt.Errorf("Expected one of the HTTP status codes %v, but got: %d", statusCodes, resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		return string(bodyBytes), nil
	})
}
