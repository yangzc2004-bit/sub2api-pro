package service

import "testing"

func TestCompareVersionsTreatsProSuffixAsSameBaseVersion(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{
			name:    "pro suffix same upstream tag",
			current: "0.1.131-pro",
			latest:  "0.1.131",
			want:    0,
		},
		{
			name:    "v prefix pro suffix same upstream tag",
			current: "v0.1.131-pro",
			latest:  "v0.1.131",
			want:    0,
		},
		{
			name:    "pro suffix still detects newer upstream",
			current: "0.1.131-pro",
			latest:  "0.1.132",
			want:    -1,
		},
		{
			name:    "pro suffix can be ahead",
			current: "0.1.132-pro",
			latest:  "0.1.131",
			want:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareVersions(tt.current, tt.latest); got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
