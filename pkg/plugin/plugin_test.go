package plugin

import (
	"slices"
	"testing"
)

// headerChars is the allowlist splitAndFilterList is called with in practice.
const headerChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ!%"

// TestSplitAndFilterList covers the strings.SplitSeq rewrite of the per-rune
// allowlist check, which is what rejects invalid --sort column names.
func TestSplitAndFilterList(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "names with descending prefix", raw: "NAME,!RESTARTS", want: []string{"NAME", "!RESTARTS"}},
		{name: "trims and upper cases", raw: " name , used ", want: []string{"NAME", "USED"}},
		{name: "rejects char outside the allowlist", raw: "bad;name", want: []string{}, wantErr: true},
		{name: "empty input", raw: "", want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := splitAndFilterList(test.raw, headerChars)

			if (err != nil) != test.wantErr {
				t.Fatalf("wantErr %v, got err %v", test.wantErr, err)
			}
			if !slices.Equal(got, test.want) {
				t.Errorf("want %#v, got %#v", test.want, got)
			}
		})
	}
}
