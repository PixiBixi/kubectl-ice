package plugin

import (
	"io"
	"testing"
)

func newRenderTestTable() *Table {
	tbl := &Table{}
	tbl.SetHeader("NAME", "COUNT")
	tbl.AddRow(NewCellText("nginx"), NewCellInt("3", 3))
	tbl.AddRow(NewCellText(""), NewCellInt("0", 0))

	return tbl
}

// TestRenderFormats pins the exact bytes of every non-text output format. The
// JSON and CSV writers do not escape quotes in cell values, so the expected
// strings here also lock in that known limitation rather than hide it.
func TestRenderFormats(t *testing.T) {
	tests := []struct {
		name   string
		render func(*Table) string
		want   string
	}{
		{
			name:   "json",
			render: (*Table).SprintJson,
			want:   "{\"data\":[\n{\"NAME\": \"nginx\", \"COUNT\": \"3\"}, \n{\"NAME\": \"\", \"COUNT\": \"0\"}\n]}\n",
		},
		{
			name:   "yaml",
			render: (*Table).SprintYaml,
			want:   "data:\n- NAME: \"nginx\"\n  COUNT: \"3\"\n- NAME: \"\"\n  COUNT: \"0\"\n",
		},
		{
			name:   "csv",
			render: (*Table).SprintCsv,
			want:   "\"NAME\", \"COUNT\"\n\"nginx\", \"3\"\n\"\", \"0\"\n",
		},
		{
			name:   "list",
			render: (*Table).SprintList,
			want:   "NAME: nginx\nCOUNT: 3\nNAME: \nCOUNT: 0\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.render(newRenderTestTable()); got != test.want {
				t.Errorf("\n want %q\n got  %q", test.want, got)
			}
		})
	}
}

func benchmarkRender(b *testing.B, rowCount int, render func(*Table, io.Writer)) {
	tbl := newSortTestTable(rowCount)

	for b.Loop() {
		render(tbl, io.Discard)
	}
}

func BenchmarkFprintJson2000(b *testing.B) { benchmarkRender(b, 2000, (*Table).FprintJson) }
func BenchmarkFprintYaml2000(b *testing.B) { benchmarkRender(b, 2000, (*Table).FprintYaml) }
func BenchmarkFprintCsv2000(b *testing.B)  { benchmarkRender(b, 2000, (*Table).FprintCsv) }
