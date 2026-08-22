package plugin

import (
	"testing"
)

// benchMatchBuilder sets a wildcard filter on the IMAGE column, the shape
// --match IMAGE=*app* produces.
func benchMatchBuilder(b *testing.B) (*RowBuilder, []Cell) {
	b.Helper()

	builder, err := makeBuilderWithFilter(
		[]string{"PODNAME", "IMAGE"},
		map[string]matchValue{"IMAGE": {operator: "==", value: "*project/app*"}},
	)
	if err != nil {
		b.Fatal(err)
	}

	row := []Cell{
		NewCellText("workload-00042-abcde-xyz12"),
		NewCellText("eu.gcr.io/project/app:v1.2.3"),
	}

	return builder, row
}

// BenchmarkMatchShouldExclude is the per row cost of --match. It runs once per
// row per render, and again on every rebuild in watch mode.
func BenchmarkMatchShouldExclude(b *testing.B) {
	builder, row := benchMatchBuilder(b)

	for b.Loop() {
		builder.matchShouldExclude(row)
	}
}

func BenchmarkStrMatch(b *testing.B) {
	cases := []struct{ str, pattern string }{
		{"eu.gcr.io/project/app:v1.2.3", "*project/app*"},
		{"workload-00042-abcde-xyz12", "workload-*-xyz??"},
		{"nats", "nats"},
		{"docker.io/istio/proxyv2:1.20.0", "*nomatch*"},
	}

	for b.Loop() {
		for _, c := range cases {
			strMatch(c.str, c.pattern)
		}
	}
}

// TestStrMatchCases pins the semantics before the implementation changes. The
// wildcard must cross a slash, which is why path.Match is not a drop in: image
// names contain slashes and --match IMAGE=*app* has to match them.
func TestStrMatchCases(t *testing.T) {
	tests := []struct {
		str     string
		pattern string
		want    bool
	}{
		{"abc", "abc", true},
		{"abc", "abd", false},
		{"", "", true},
		{"", "*", true},
		{"abc", "*", true},
		{"abc", "a*", true},
		{"abc", "*c", true},
		{"abc", "a*c", true},
		{"abc", "a?c", true},
		{"abc", "a?", false},
		{"abc", "?bc", true},
		{"abc", "abc*", true},
		{"abc", "*abc*", true},
		{"eu.gcr.io/project/app:v1", "*project/app*", true},
		{"eu.gcr.io/project/app:v1", "*/app*", true},
		{"eu.gcr.io/other/app:v1", "*project*", false},
		{"a", "", false},
		{"", "?", false},
		{"aaa", "*a", true},
		{"aaa", "a*a", true},
		{"abcdef", "a*c*f", true},
		{"abcdef", "a*c*g", false},
		{"xy", "**", true},
		{"xy", "?*?", true},
		{"x", "?*?", false},
	}

	for _, test := range tests {
		if got := strMatch(test.str, test.pattern); got != test.want {
			t.Errorf("strMatch(%q, %q) = %v, want %v", test.str, test.pattern, got, test.want)
		}
	}
}

// strMatchDP is the dynamic programming implementation strMatch used before it
// became allocation free. Kept as the reference oracle.
func strMatchDP(str string, pattern string) bool {
	// shamelessly converted from c++ code on web as I was too lazy to work it out myself
	// source: https://www.geeksforgeeks.org/wildcard-pattern-matching/

	n := len(str)
	m := len(pattern)

	if m == 0 {
		return (n == 0)
	}

	lookup := make([][]bool, n+1)
	for i := range lookup {
		lookup[i] = make([]bool, m+1)
	}

	lookup[0][0] = true

	for i, char := range pattern {
		j := i + 1
		if char == '*' {
			lookup[0][j] = lookup[0][j-1]
		}
	}

	for q, s := range str {
		i := q + 1
		for w, char := range pattern {
			j := w + 1
			switch {
			case char == '*':
				lookup[i][j] = lookup[i][j-1] || lookup[i-1][j]
			case char == '?' || s == char:
				lookup[i][j] = lookup[i-1][j-1]
			default:
				lookup[i][j] = false
			}
		}
	}
	return lookup[n][m]
}

// TestStrMatchMatchesOldImplementation walks a small alphabet exhaustively so
// the rewrite has to agree with the dynamic programming version on every short
// input, not only on the cases someone thought of.
func TestStrMatchMatchesOldImplementation(t *testing.T) {
	expand := func(runes []rune, maxLen int) []string {
		var out []string
		var rec func(prefix string)
		rec = func(prefix string) {
			out = append(out, prefix)
			if len(prefix) == maxLen {
				return
			}
			for _, r := range runes {
				rec(prefix + string(r))
			}
		}
		rec("")
		return out
	}

	strs := expand([]rune{'a', 'b', '/'}, 4)
	patterns := expand([]rune{'a', 'b', '/', '*', '?'}, 4)
	t.Logf("%d strings x %d patterns = %d combinations", len(strs), len(patterns), len(strs)*len(patterns))

	for _, str := range strs {
		for _, pattern := range patterns {
			if got, want := strMatch(str, pattern), strMatchDP(str, pattern); got != want {
				t.Fatalf("strMatch(%q, %q) = %v, the old implementation says %v", str, pattern, got, want)
			}
		}
	}
}

// TestStrMatchUTF8 covers multi byte input, which the dynamic programming
// version got wrong: it sized its table with len in bytes but indexed it with
// the byte offsets range yields over a string, so rows were skipped and
// strMatchDP("café", "café") returned false. Kubernetes names are ASCII so
// nobody hit it, but a --match on an annotation value could.
func TestStrMatchUTF8(t *testing.T) {
	tests := []struct {
		str     string
		pattern string
		want    bool
	}{
		{"café", "café", true},
		{"café", "caf*", true},
		{"café", "caf?", false}, // ? matches one byte, é is two
		{"héllo/wörld", "*/w*", true},
		{"héllo", "h*o", true},
		{"héllo", "h*x", false},
	}

	for _, test := range tests {
		if got := strMatch(test.str, test.pattern); got != test.want {
			t.Errorf("strMatch(%q, %q) = %v, want %v", test.str, test.pattern, got, test.want)
		}
	}

	// the specific regression the rewrite fixes
	if strMatchDP("café", "café") {
		t.Error("the old implementation now matches a multi byte string against itself, update this test")
	}
}
