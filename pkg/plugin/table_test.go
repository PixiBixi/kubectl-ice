package plugin

import (
	"reflect"
	"strings"
	"testing"
)

var table Table

// *****************
// SetHeader
// *****************
type tableSetHeaderTest struct {
	arg1        []string
	count       int
	columnorder []int
	expected    []headerRow
}

var tableSetHeaderTests = []tableSetHeaderTest{
	{[]string{"a", "b"}, 2, []int{0, 1}, []headerRow{{3, 0, false, 0, "a"}, {3, 0, false, 0, "b"}}},
}

func TestTableSetHeader(t *testing.T) {

	for _, test := range tableSetHeaderTests {

		table.SetHeader(test.arg1...)
		if table.headCount != test.count {
			t.Errorf("Output %v not equal to expected \"%v\"", table.headCount, test.count)
		}
		if !reflect.DeepEqual(table.head, test.expected) {
			t.Errorf("Output %v not equal to expected \"%v\"", table.head, test.expected)
		}
		if !reflect.DeepEqual(table.columnOrder, test.columnorder) {
			t.Errorf("Output %v not equal to expected \"%v\"", table.columnOrder, test.columnorder)
		}
	}

}

// *****************
// AddRow
// *****************
type addRowTest struct {
	arg1      []Cell
	rowCount  int
	columnLen int
	expected  [][]Cell
}

var addRowTests = []addRowTest{
	{[]Cell{NewCellText("one")}, 1, 5, [][]Cell{{Cell{"one", 0, 0, 0, 0, 0, [2]int{-1, 0}}}}},
	{[]Cell{NewCellText("two")}, 2, 5, [][]Cell{{Cell{"one", 0, 0, 0, 0, 0, [2]int{-1, 0}}}, {Cell{"two", 0, 0, 0, 0, 0, [2]int{-1, 0}}}}},
	{[]Cell{NewCellText("three")}, 3, 7, [][]Cell{{Cell{"one", 0, 0, 0, 0, 0, [2]int{-1, 0}}}, {Cell{"two", 0, 0, 0, 0, 0, [2]int{-1, 0}}}, {Cell{"three", 0, 0, 0, 0, 0, [2]int{-1, 0}}}}},
	{[]Cell{NewCellText("four"), NewCellText("extra"), NewCellText("larger")}, 4, 7, [][]Cell{{Cell{"one", 0, 0, 0, 0, 0, [2]int{-1, 0}}}, {Cell{"two", 0, 0, 0, 0, 0, [2]int{-1, 0}}}, {Cell{"three", 0, 0, 0, 0, 0, [2]int{-1, 0}}}, {Cell{"four", 0, 0, 0, 0, 0, [2]int{-1, 0}}, Cell{"extra", 0, 0, 0, 0, 0, [2]int{-1, 0}}, Cell{"larger", 0, 0, 0, 0, 0, [2]int{-1, 0}}}}},
}

func TestAddRow(t *testing.T) {
	table.SetHeader("A")

	for _, test := range addRowTests {

		table.AddRow(test.arg1...)
		if table.currentRow != test.rowCount {
			t.Errorf("Output %v not equal to expected \"%v\"", table.currentRow, test.rowCount)
		}
		if table.head[0].columnLength != test.columnLen {
			t.Errorf("Output %v not equal to expected \"%v\"", table.head[0].columnLength, test.columnLen)
		}
		if !reflect.DeepEqual(table.data, test.expected) {
			t.Errorf("Output %v not equal to expected \"%v\"", table.data, test.expected)
		}
	}

}

// *****************
// Order
// *****************
type orderTest struct {
	arg1     []int
	expected []int
}

var orderTests = []orderTest{
	{[]int{0, 1}, []int{0, 1, 2}},
	{[]int{1, 0}, []int{1, 0, 2}},
	{[]int{2, 1}, []int{2, 1, 0}},
	{[]int{2, 0, 1}, []int{2, 0, 1}},
	{[]int{3, 0, 1, 3}, []int{3, 0, 1, 3, 2}},
}

func TestOrder(t *testing.T) {
	table.SetHeader("A", "B", "C")

	for _, test := range orderTests {
		table.Order(test.arg1...)
		if !reflect.DeepEqual(table.columnOrder, test.expected) {
			t.Errorf("Output %v not equal to expected \"%v\"", table.columnOrder, test.expected)
		}
	}

}

// *****************
// HideColumn
// *****************
type hideColumnTest struct {
	arg1     int
	expected []headerRow
}

// []headerRow{{ 3, 0, false, 0, "a"}, { 3, 0, false, 0, "b"}}}
var hideColumnTests = []hideColumnTest{
	{2, []headerRow{{3, 0, false, 0, "A"}, {3, 0, false, 0, "B"}, {3, 0, true, 0, "C"}}},
	{2, []headerRow{{3, 0, false, 0, "A"}, {3, 0, false, 0, "B"}, {3, 0, true, 0, "C"}}},
	{0, []headerRow{{3, 0, true, 0, "A"}, {3, 0, false, 0, "B"}, {3, 0, true, 0, "C"}}},
}

func TestHideColumn(t *testing.T) {
	table.SetHeader("A", "B", "C")

	for _, test := range hideColumnTests {
		table.HideColumn(test.arg1)
		if !reflect.DeepEqual(table.head, test.expected) {
			t.Errorf("Output %v not equal to expected \"%v\"", table.head, test.expected)
		}
	}

}

// *****************
// Sprint / Fprint (strings.Builder path)
// *****************

func TestTableSprint(t *testing.T) {
	tbl := Table{}
	tbl.SetHeader("NAME", "STATUS")
	tbl.AddRow(NewCellText("pod-a"), NewCellText("Running"))
	tbl.AddRow(NewCellText("pod-b"), NewCellText("Pending"))

	out := tbl.Sprint()

	if !strings.Contains(out, "NAME") {
		t.Error("output missing header NAME")
	}
	if !strings.Contains(out, "pod-a") {
		t.Error("output missing row pod-a")
	}
	if !strings.Contains(out, "pod-b") {
		t.Error("output missing row pod-b")
	}
	if !strings.Contains(out, "Running") {
		t.Error("output missing value Running")
	}
}

func TestTableSprintMultipleCalls(t *testing.T) {
	// Sprint must be idempotent — calling it twice should return the same output
	tbl := Table{}
	tbl.SetHeader("A", "B")
	tbl.AddRow(NewCellText("x"), NewCellText("y"))

	first := tbl.Sprint()
	second := tbl.Sprint()

	if first != second {
		t.Error("Sprint() not idempotent — consecutive calls returned different output")
	}
}

func TestTableSprintHiddenColumn(t *testing.T) {
	tbl := Table{}
	tbl.SetHeader("VISIBLE", "HIDDEN")
	tbl.HideColumn(1)
	tbl.AddRow(NewCellText("show"), NewCellText("hide"))

	out := tbl.Sprint()

	if !strings.Contains(out, "show") {
		t.Error("visible column value missing from output")
	}
	if strings.Contains(out, "hide") {
		t.Error("hidden column value should not appear in output")
	}
}

func TestTableSprintUnicode(t *testing.T) {
	// Verify utf8.RuneCountInString path: unicode chars must not break column alignment
	tbl := Table{}
	tbl.SetHeader("NAME")
	tbl.AddRow(NewCellText("日本語テスト"))
	tbl.AddRow(NewCellText("ascii"))

	out := tbl.Sprint()
	if !strings.Contains(out, "日本語テスト") {
		t.Error("unicode value missing from output")
	}
	if !strings.Contains(out, "ascii") {
		t.Error("ascii value missing from output")
	}
}

func TestHideColumnPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()

	// The following is the code under test
	table.HideColumn(4)
}
