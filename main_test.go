package main

import (
	"flag"
	"os"
	"testing"
	"time"

	"github.com/ianschenck/envflag"

	"github.com/linode/linode-blockstorage-csi-driver/internal/driver"
)

// withFreshEnvFlags lets loadConfig be called more than once per test binary:
// it registers its flags on the global envflag set, which panics on redefinition.
func withFreshEnvFlags(t *testing.T) {
	t.Helper()
	old := envflag.EnvironmentFlags
	envflag.EnvironmentFlags = flag.NewFlagSet("environment", flag.ContinueOnError)
	t.Cleanup(func() { envflag.EnvironmentFlags = old })
}

// unsetEnv removes an environment variable for the duration of the test.
// t.Setenv cannot unset, and an empty value would still be parsed by envflag.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		}
	})
}

func TestLoadConfigMetadataService(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		wantSkip    bool
		wantTimeout time.Duration
	}{
		{
			name:        "defaults",
			wantSkip:    false,
			wantTimeout: 5 * time.Second,
		},
		{
			name: "skip metadata service with custom timeout",
			env: map[string]string{
				"LINODE_SKIP_METADATA_SERVICE":    "true",
				"LINODE_METADATA_SERVICE_TIMEOUT": "250ms",
			},
			wantSkip:    true,
			wantTimeout: 250 * time.Millisecond,
		},
		{
			name: "zero timeout disables the bound",
			env: map[string]string{
				"LINODE_SKIP_METADATA_SERVICE":    "false",
				"LINODE_METADATA_SERVICE_TIMEOUT": "0s",
			},
			wantSkip:    false,
			wantTimeout: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFreshEnvFlags(t)
			unsetEnv(t, "LINODE_SKIP_METADATA_SERVICE")
			unsetEnv(t, "LINODE_METADATA_SERVICE_TIMEOUT")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg := loadConfig()

			if cfg.skipMetadataService != tt.wantSkip {
				t.Errorf("skipMetadataService = %v, want %v", cfg.skipMetadataService, tt.wantSkip)
			}
			if cfg.metadataServiceTimeout != tt.wantTimeout {
				t.Errorf("metadataServiceTimeout = %v, want %v", cfg.metadataServiceTimeout, tt.wantTimeout)
			}
		})
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	withFreshEnvFlags(t)
	for _, key := range []string{"DRIVER_ROLE", "CSI_ENDPOINT", "METRICS_PORT", "OTEL_TRACING_PORT", "LINODE_LIST_VOLUMES_FILTER_BY_PREFIX"} {
		unsetEnv(t, key)
	}

	cfg := loadConfig()

	if cfg.driverRole != "controller" {
		t.Errorf("driverRole = %q, want %q", cfg.driverRole, "controller")
	}
	if cfg.csiEndpoint != "unix:/tmp/csi.sock" {
		t.Errorf("csiEndpoint = %q, want %q", cfg.csiEndpoint, "unix:/tmp/csi.sock")
	}
	if cfg.metricsPort != "8081" || cfg.tracingPort != "4318" {
		t.Errorf("metricsPort, tracingPort = %q, %q, want %q, %q", cfg.metricsPort, cfg.tracingPort, "8081", "4318")
	}
	if cfg.listVolumesFilterByPrefix {
		t.Error("listVolumesFilterByPrefix = true, want false")
	}
}

func TestConfigurationMetadataOptions(t *testing.T) {
	cfg := configuration{skipMetadataService: true, metadataServiceTimeout: 3 * time.Second}

	got := cfg.metadataOptions()

	want := driver.MetadataOptions{SkipMetadataService: true, MetadataServiceTimeout: 3 * time.Second}
	if got != want {
		t.Errorf("metadataOptions() = %+v, want %+v", got, want)
	}
}
