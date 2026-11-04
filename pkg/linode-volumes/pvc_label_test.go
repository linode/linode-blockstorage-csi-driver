package linodevolumes

import (
	"fmt"
	"strings"
	"testing"
)

func TestPVCVolumeLabel(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		namespace  string
		pvc        string
		volumeName string
		want       string
		wantOK     bool
	}{
		{
			name:       "short names fit unchanged",
			namespace:  "default",
			pvc:        "data-postgres-0",
			volumeName: "pvc-72af5c8ff02c4813",
			want:       "default-data-postgres-0-f02c4813",
			wantOK:     true,
		},
		{
			name:       "with driver prefix",
			prefix:     "prod-",
			namespace:  "db",
			pvc:        "pg-1",
			volumeName: "pvc-72af5c8ff02c4813",
			want:       "prod-db-pg-1-f02c4813",
			wantOK:     true,
		},
		{
			name:       "long names are truncated in the middle and keep the ordinal",
			namespace:  "llz-openbao",
			pvc:        "data-platform-openbao-2",
			volumeName: "pvc-0123456789abcdef",
			want:       "llz-openbao-m-openbao-2-89abcdef",
			wantOK:     true,
		},
		{
			name:       "dots and repeated separators are normalized",
			namespace:  "team--a",
			pvc:        "cache.redis__0",
			volumeName: "pvc-0123456789abcdef",
			want:       "team-a-cache-redis_0-89abcdef",
			wantOK:     true,
		},
		{
			name:       "full-length uid with dashes",
			namespace:  "default",
			pvc:        "www",
			volumeName: "pvc-6f1c2e1a-3b7d-4c2e-9f00-1a2b3c4d5e6f",
			want:       "default-www-3c4d5e6f",
			wantOK:     true,
		},
		{
			name:       "namespace starting with a digit is rejected",
			namespace:  "1team",
			pvc:        "data",
			volumeName: "pvc-0123456789abcdef",
			wantOK:     false,
		},
		{
			name:       "missing namespace",
			pvc:        "data",
			volumeName: "pvc-0123456789abcdef",
			wantOK:     false,
		},
		{
			name:       "missing pvc name",
			namespace:  "default",
			volumeName: "pvc-0123456789abcdef",
			wantOK:     false,
		},
		{
			name:       "volume name without letters or digits",
			namespace:  "default",
			pvc:        "data",
			volumeName: "---",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PVCVolumeLabel(tt.prefix, tt.namespace, tt.pvc, tt.volumeName)
			if ok != tt.wantOK {
				t.Fatalf("PVCVolumeLabel() ok = %v, want %v (label %q)", ok, tt.wantOK, got)
			}
			if got != tt.want {
				t.Errorf("PVCVolumeLabel() = %q, want %q", got, tt.want)
			}
			if ok && !isValidVolumeLabel(got) {
				t.Errorf("PVCVolumeLabel() returned invalid label %q", got)
			}
		})
	}
}

// StatefulSet replicas with long names used to collapse to the same label
// once truncated, and Linode rejects duplicate labels.
func TestPVCVolumeLabel_StatefulSetOrdinalsStayDistinct(t *testing.T) {
	seen := map[string]int{}
	for i := 0; i < 12; i++ {
		pvc := fmt.Sprintf("data-platform-openbao-%d", i)
		volumeName := fmt.Sprintf("pvc-%016x", 0x72af5c8ff02c4800+i)
		label, ok := PVCVolumeLabel("", "llz-openbao", pvc, volumeName)
		if !ok {
			t.Fatalf("no label for %s", pvc)
		}
		if prev, dup := seen[label]; dup {
			t.Fatalf("replicas %d and %d both got label %q", prev, i, label)
		}
		seen[label] = i
		if !strings.Contains(label, fmt.Sprintf("openbao-%d-", i)) {
			t.Errorf("label %q lost the ordinal %d", label, i)
		}
	}
}

// A PVC that is deleted and recreated with the same name gets a new UID, so
// it must not resolve to the label of a volume that was retained from the
// old claim.
func TestPVCVolumeLabel_RecreatedPVCGetsNewLabel(t *testing.T) {
	a, okA := PVCVolumeLabel("", "default", "data", "pvc-1111111122222222")
	b, okB := PVCVolumeLabel("", "default", "data", "pvc-1111111133333333")
	if !okA || !okB {
		t.Fatal("expected labels")
	}
	if a == b {
		t.Fatalf("recreated PVC reused label %q", a)
	}
}

func TestPVCVolumeLabel_AlwaysValid(t *testing.T) {
	prefixes := []string{"", "a", "csi-", "lke123-", "abcdefghijkl", "x_"}
	names := []string{"a", "data", "x.y.z", "-lead", "trail-", strings.Repeat("n", 253), "a--b__c..d", "data-platform-openbao-0"}
	for _, prefix := range prefixes {
		for _, ns := range []string{"default", "kube-system", strings.Repeat("k", 63)} {
			for _, pvc := range names {
				label, ok := PVCVolumeLabel(prefix, ns, pvc, "pvc-0123456789abcdef")
				if !ok {
					t.Errorf("no label for prefix=%q ns=%q pvc=%q", prefix, ns, pvc)
					continue
				}
				if len(label) > LinodeVolumeLabelLength || !isValidVolumeLabel(label) {
					t.Errorf("invalid label %q (len %d) for prefix=%q ns=%q pvc=%q", label, len(label), prefix, ns, pvc)
				}
				if !strings.HasSuffix(label, "-89abcdef") {
					t.Errorf("label %q lost the uid suffix", label)
				}
				// The label must round-trip through the CSI volume handle unchanged.
				created := CreateLinodeVolumeKey(42, label)
				key, err := ParseLinodeVolumeKey(created.GetVolumeKey())
				if err != nil || key.GetNormalizedLabel() != label {
					t.Errorf("label %q does not round-trip through the volume key: %v", label, err)
				}
			}
		}
	}
}
