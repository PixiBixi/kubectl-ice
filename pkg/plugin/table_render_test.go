package plugin

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
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

// newEscapeTestTable holds the characters that used to be written out raw and
// broke every downstream parser.
func newEscapeTestTable() *Table {
	tbl := &Table{}
	tbl.SetHeader("NAME", "MESSAGE")
	tbl.AddRow(NewCellText(`say "hello"`), NewCellText(`back\slash`))
	tbl.AddRow(NewCellText("plain"), NewCellText("comma, inside"))

	return tbl
}

// TestSprintJsonIsValidJson proves the json writer emits parseable json even when
// cells contain quotes or backslashes, which previously produced broken output.
func TestSprintJsonIsValidJson(t *testing.T) {
	var got struct {
		Data []map[string]string `json:"data"`
	}

	out := newEscapeTestTable().SprintJson()
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid json: %v\n%s", err, out)
	}

	if len(got.Data) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got.Data))
	}
	if want := `say "hello"`; got.Data[0]["NAME"] != want {
		t.Errorf("NAME: want %q, got %q", want, got.Data[0]["NAME"])
	}
	if want := `back\slash`; got.Data[0]["MESSAGE"] != want {
		t.Errorf("MESSAGE: want %q, got %q", want, got.Data[0]["MESSAGE"])
	}
}

// TestSprintCsvIsValidCsv proves the csv writer doubles embedded quotes. The
// reader needs TrimLeadingSpace because the writer separates fields with ", ".
func TestSprintCsvIsValidCsv(t *testing.T) {
	reader := csv.NewReader(strings.NewReader(newEscapeTestTable().SprintCsv()))
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("output is not valid csv: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("want header plus 2 rows, got %d records", len(records))
	}
	if want := `say "hello"`; records[1][0] != want {
		t.Errorf("NAME: want %q, got %q", want, records[1][0])
	}
	if want := "comma, inside"; records[2][1] != want {
		t.Errorf("MESSAGE: want %q, got %q", want, records[2][1])
	}
}

// TestSprintYamlIsValidYaml proves the yaml writer quotes values the same way,
// since a raw quote inside a double quoted scalar breaks the document.
func TestSprintYamlIsValidYaml(t *testing.T) {
	var got struct {
		Data []map[string]string `json:"data"`
	}

	out := newEscapeTestTable().SprintYaml()
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid yaml: %v\n%s", err, out)
	}

	if len(got.Data) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got.Data))
	}
	if want := `say "hello"`; got.Data[0]["NAME"] != want {
		t.Errorf("NAME: want %q, got %q", want, got.Data[0]["NAME"])
	}
}
