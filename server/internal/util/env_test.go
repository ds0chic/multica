package util

import "testing"

func TestEnvBool(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		def  bool
		want bool
	}{
		{"", true, true},
		{"", false, false},
		{"true", false, true},
		{" TRUE ", false, true},
		{"1", false, true},
		{"yes", false, true},
		{"On", false, true},
		{"false", true, false},
		{"0", true, false},
		{"no", true, false},
		{"OFF", true, false},
		{"maybe", true, true},
		{"maybe", false, false},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			t.Setenv("MULTICA_TEST_BOOL", tt.raw)
			if got := EnvBool("MULTICA_TEST_BOOL", tt.def); got != tt.want {
				t.Fatalf("EnvBool(%q, %v) = %v, want %v", tt.raw, tt.def, got, tt.want)
			}
		})
	}
}
