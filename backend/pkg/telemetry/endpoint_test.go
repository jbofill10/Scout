package telemetry

import "testing"

func TestNormalizeOTLPEndpoint(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		wantHost     string
		wantInsecure bool
	}{
		{"bare host:port", "localhost:4317", "localhost:4317", true},
		{"http scheme", "http://192.168.0.111:4317", "192.168.0.111:4317", true},
		{"https scheme", "https://collector.example.com:4317", "collector.example.com:4317", false},
		{"http scheme with path", "http://192.168.0.111:4317/v1/traces", "192.168.0.111:4317", true},
		{"bare host:port with path", "collector:4317/v1/logs", "collector:4317", true},
		{"empty", "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, insecure := normalizeOTLPEndpoint(tc.in)
			if host != tc.wantHost {
				t.Errorf("host = %q, want %q", host, tc.wantHost)
			}
			if insecure != tc.wantInsecure {
				t.Errorf("insecure = %v, want %v", insecure, tc.wantInsecure)
			}
		})
	}
}
