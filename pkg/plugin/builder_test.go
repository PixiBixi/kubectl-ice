package plugin

import (
	"testing"
)

// ***********************
// setFilter + matchShouldExclude
// ***********************

func makeBuilderWithFilter(headers []string, filterList map[string]matchValue) (*RowBuilder, error) {
	b := &RowBuilder{
		FilterList: filterList,
	}
	b.head = headers
	b.filter = make([]matchFilter, len(headers))
	if err := b.setFilter(filterList); err != nil {
		return nil, err
	}
	return b, nil
}

func TestSetFilterPreParsesIntValue(t *testing.T) {
	b, err := makeBuilderWithFilter(
		[]string{"CPU"},
		map[string]matchValue{"CPU": {operator: ">", value: "42"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if b.filter[0].valueInt != 42 {
		t.Errorf("valueInt = %d, want 42", b.filter[0].valueInt)
	}
	if b.filter[0].valueFloat != 42.0 {
		t.Errorf("valueFloat = %f, want 42.0", b.filter[0].valueFloat)
	}
}

func TestSetFilterPreParsesFloatValue(t *testing.T) {
	b, err := makeBuilderWithFilter(
		[]string{"PCT"},
		map[string]matchValue{"PCT": {operator: "<", value: "75.5"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if b.filter[0].valueFloat != 75.5 {
		t.Errorf("valueFloat = %f, want 75.5", b.filter[0].valueFloat)
	}
}

func TestSetFilterInvalidColumn(t *testing.T) {
	b := &RowBuilder{FilterList: map[string]matchValue{"MISSING": {operator: "=", value: "x"}}}
	b.head = []string{"COL1"}
	b.filter = make([]matchFilter, 1)
	if err := b.setFilter(b.FilterList); err == nil {
		t.Error("expected error for unknown column, got nil")
	}
}

func TestSetFilterInvalidOperator(t *testing.T) {
	b := &RowBuilder{FilterList: map[string]matchValue{"COL1": {operator: "~=", value: "x"}}}
	b.head = []string{"COL1"}
	b.filter = make([]matchFilter, 1)
	if err := b.setFilter(b.FilterList); err == nil {
		t.Error("expected error for invalid operator, got nil")
	}
}

// matchShouldExclude — int filter
func TestMatchShouldExcludeInt(t *testing.T) {
	b, err := makeBuilderWithFilter(
		[]string{"VAL"},
		map[string]matchValue{"VAL": {operator: ">", value: "50"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	// 100 > 50: row should be included
	if b.matchShouldExclude([]Cell{NewCellInt("100", 100)}) {
		t.Error("row with value 100 should be included by filter >50")
	}
	// 30 is not > 50: row should be excluded
	if !b.matchShouldExclude([]Cell{NewCellInt("30", 30)}) {
		t.Error("row with value 30 should be excluded by filter >50")
	}
}

// matchShouldExclude — string filter
func TestMatchShouldExcludeString(t *testing.T) {
	b, err := makeBuilderWithFilter(
		[]string{"STATUS"},
		map[string]matchValue{"STATUS": {operator: "!=", value: "True"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	// "False" != "True": included
	if b.matchShouldExclude([]Cell{NewCellText("False")}) {
		t.Error("row with 'False' should be included by filter !=True")
	}
	// "True" == "True": excluded
	if !b.matchShouldExclude([]Cell{NewCellText("True")}) {
		t.Error("row with 'True' should be excluded by filter !=True")
	}
}

// matchShouldExclude — float filter
func TestMatchShouldExcludeFloat(t *testing.T) {
	b, err := makeBuilderWithFilter(
		[]string{"PCT"},
		map[string]matchValue{"PCT": {operator: ">", value: "75.0"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	// 80.0 > 75.0: included
	if b.matchShouldExclude([]Cell{NewCellFloat("80.0", 80.0)}) {
		t.Error("row with 80.0 should be included by filter >75.0")
	}
	// 50.0 is not > 75.0: excluded
	if !b.matchShouldExclude([]Cell{NewCellFloat("50.0", 50.0)}) {
		t.Error("row with 50.0 should be excluded by filter >75.0")
	}
}

// matchShouldExclude — no filter set: never exclude
func TestMatchShouldExcludeNoFilter(t *testing.T) {
	b := &RowBuilder{}
	if b.matchShouldExclude([]Cell{NewCellText("anything")}) {
		t.Error("should not exclude when FilterList is empty")
	}
}
