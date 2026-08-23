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

// TestColourFromEnvironment pins which spelling of the colour environment
// variable is honoured. Users arriving from upstream kubectl-ice have ICE_COLOUR
// in their shell profile, and the flag help text advertised that spelling for
// both projects, so both have to work.
func TestColourFromEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		colour   string
		colourUK string
		flag     string
		want     int
	}{
		{name: "ICE_COLOR is honoured", colour: "errors", want: COLOUR_ERRORS},
		{name: "ICE_COLOUR is honoured", colourUK: "errors", want: COLOUR_ERRORS},
		{name: "ICE_COLOR wins over ICE_COLOUR", colour: "errors", colourUK: "columns", want: COLOUR_ERRORS},
		{name: "the flag wins over both", colour: "columns", colourUK: "columns", flag: "errors", want: COLOUR_ERRORS},
		{name: "neither set leaves colour off", want: COLOUR_NONE},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ICE_COLOR", test.colour)
			t.Setenv("ICE_COLOUR", test.colourUK)

			flags := []string{}
			if test.flag != "" {
				flags = append(flags, "--color", test.flag)
			}

			got, err := processCommonFlags(runTestCommand(t, "status", flags...))
			if err != nil {
				t.Fatalf("processCommonFlags: %v", err)
			}
			if got.outputAsColour != test.want {
				t.Errorf("outputAsColour = %d, want %d", got.outputAsColour, test.want)
			}
		})
	}
}
