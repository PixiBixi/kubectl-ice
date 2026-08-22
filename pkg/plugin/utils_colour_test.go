package plugin

import (
	"slices"
	"testing"
)

// TestSplitColourString covers the --color=custom parser. The two length checks
// are defensive: the only caller filters empty and short segments before calling
// in, but the function used to panic on index out of range rather than return an
// error, which made it unsafe for any future caller.
func TestSplitColourString(t *testing.T) {
	tests := []struct {
		in         string
		wantPrefix string
		wantCode   int
		wantMod    int
		wantErr    bool
	}{
		{in: "1.31", wantCode: 31, wantMod: 1},
		{in: "0.32", wantCode: 32, wantMod: 0},
		{in: "g1.31", wantPrefix: "g", wantCode: 31, wantMod: 1},
		{in: "w0.33", wantPrefix: "w", wantCode: 33, wantMod: 0},
		{in: "b1.30", wantPrefix: "b", wantCode: 30, wantMod: 1},
		{in: "", wantErr: true},
		{in: "31", wantErr: true},
		{in: "x", wantErr: true},
		{in: ".", wantErr: true},
		{in: "1.x", wantErr: true},
	}

	for _, test := range tests {
		prefix, code, mod, err := splitColourString(test.in)

		if (err != nil) != test.wantErr {
			t.Errorf("splitColourString(%q) error = %v, wantErr %v", test.in, err, test.wantErr)
			continue
		}
		if test.wantErr {
			continue
		}
		if prefix != test.wantPrefix || code != test.wantCode || mod != test.wantMod {
			t.Errorf("splitColourString(%q) = (%q, %d, %d), want (%q, %d, %d)",
				test.in, prefix, code, mod, test.wantPrefix, test.wantCode, test.wantMod)
		}
	}
}

func TestGetColourSetFromString(t *testing.T) {
	// getColourSetFromString writes the g, w and b prefixes into package level
	// vars, so a case using them leaks into every later test. Restore them.
	okBefore, warnBefore, badBefore := colourOk, colourWarn, colourBad
	t.Cleanup(func() {
		colourOk, colourWarn, colourBad = okBefore, warnBefore, badBefore
	})

	tests := []struct {
		name       string
		in         []string
		wantColour [][2]int
		wantSet    int
		wantErr    bool
	}{
		{
			name:       "plain colours",
			in:         []string{"1.31", "0.32"},
			wantColour: [][2]int{{31, 1}, {32, 0}},
			wantSet:    COLOUR_CUSTOM,
		},
		{
			name: "a semantic prefix switches to the mixed set",
			in:   []string{"g1.32", "1.31"},
			// the g entry sets the good colour rather than joining the wheel
			wantColour: [][2]int{{31, 1}},
			wantSet:    COLOUR_CUSTOMMIX,
		},
		{
			name:       "empty segments are skipped",
			in:         []string{"", "1.31", ""},
			wantColour: [][2]int{{31, 1}},
			wantSet:    COLOUR_CUSTOM,
		},
		{
			name:       "no usable colour falls back to none",
			in:         []string{"g1.32"},
			wantColour: [][2]int{{colourNone, 0}},
			wantSet:    COLOUR_CUSTOMMIX,
		},
		{name: "too short to be a colour", in: []string{"31"}, wantErr: true},
		{name: "not a number", in: []string{"1.xx"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, set, err := getColourSetFromString(test.in)

			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if !slices.Equal(got, test.wantColour) {
				t.Errorf("colours = %v, want %v", got, test.wantColour)
			}
			if set != test.wantSet {
				t.Errorf("colourset = %d, want %d", set, test.wantSet)
			}
		})
	}
}

func TestSprintTableAs(t *testing.T) {
	table := Table{}
	table.SetHeader("NAME", "COUNT")
	table.AddRow(NewCellText("nginx"), NewCellInt("3", 3))

	tests := []struct {
		outType string
		want    string
	}{
		{outType: "json", want: "{\"data\":[\n{\"NAME\": \"nginx\", \"COUNT\": \"3\"}\n]}\n"},
		{outType: "yaml", want: "data:\n- NAME: \"nginx\"\n  COUNT: \"3\"\n"},
		{outType: "csv", want: "\"NAME\", \"COUNT\"\n\"nginx\", \"3\"\n"},
		{outType: "list", want: "NAME: nginx\nCOUNT: 3\n"},
		{outType: "", want: "NAME  COUNT\nnginx 3\n"},
	}

	for _, test := range tests {
		if got := sprintTableAs(table, test.outType); got != test.want {
			t.Errorf("sprintTableAs(%q) = %q, want %q", test.outType, got, test.want)
		}
	}
}
