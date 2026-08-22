package plugin

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

// sortBubble is the O(n^2) implementation Table.sort used before it moved to
// slices.SortStableFunc. Kept as the reference oracle for TestSortMatchesBubbleSort.
func (t *Table) sortBubble(list []int, columnNumber int, ascending bool) {
	for i := 0; i < t.currentRow+1; i++ {
		hasMoved := false
		for j := 0; j < t.currentRow-1; j++ {
			var wordLow, wordHigh string
			var intLow, intHigh int64
			var floatHigh, floatLow float64

			switchOrder := false
			jLow := list[j]
			jHigh := list[j+1]

			switch t.data[jLow][columnNumber].typ {
			case 0:
				wordLow = t.data[jLow][columnNumber].text
				wordHigh = t.data[jHigh][columnNumber].text
			case 1:
				intLow = t.data[jLow][columnNumber].number
				intHigh = t.data[jHigh][columnNumber].number
			case 2:
				floatLow = t.data[jLow][columnNumber].float
				floatHigh = t.data[jHigh][columnNumber].float
			}

			if ascending {
				switch t.data[jLow][columnNumber].typ {
				case 0:
					if wordLow > wordHigh {
						switchOrder = true
					}
				case 1:
					if intLow > intHigh {
						switchOrder = true
					}
				case 2:
					if floatLow > floatHigh {
						switchOrder = true
					}
				}
			} else {
				switch t.data[jLow][columnNumber].typ {
				case 0:
					if wordLow < wordHigh {
						switchOrder = true
					}
				case 1:
					if intLow < intHigh {
						switchOrder = true
					}
				case 2:
					if floatLow < floatHigh {
						switchOrder = true
					}
				}
			}

			if switchOrder {
				hasMoved = true
				list[j] = jHigh
				list[j+1] = jLow
			}
		}
		if !hasMoved {
			break
		}
	}
}

// newSortTestTable builds a table of rowCount rows holding one column of every
// cell type, filled in a deterministic non-sorted order.
func newSortTestTable(rowCount int) *Table {
	tbl := &Table{}
	tbl.SetHeader("NAME", "COUNT", "USED", "PLACEHOLDER")

	for i := range rowCount {
		// spread values so neither ascending nor descending is the input order
		spread := (i*7919 + 13) % max(rowCount, 1)
		tbl.AddRow(
			NewCellText(fmt.Sprintf("pod-%06d-container", spread)),
			NewCellInt(fmt.Sprintf("%d", spread), int64(spread)),
			NewCellFloat(fmt.Sprintf("%f", float64(spread)), float64(spread)),
			NewCellEmpty(),
		)
	}

	return tbl
}

func TestSortMatchesBubbleSort(t *testing.T) {
	for _, rowCount := range []int{0, 1, 2, 3, 17, 200, 1500} {
		for column := range 4 {
			for _, ascending := range []bool{true, false} {
				tbl := newSortTestTable(rowCount)

				want := slices.Clone(tbl.rowOrder)
				got := slices.Clone(tbl.rowOrder)

				tbl.sortBubble(want, column, ascending)
				tbl.sort(got, column, ascending)

				if !slices.Equal(want, got) {
					t.Errorf("rows=%d column=%d ascending=%v\n want %v\n got  %v",
						rowCount, column, ascending, want, got)
				}
			}
		}
	}
}

// TestSortByNamesIsCumulative checks the secondary column keeps its order within
// equal primary values, which SortByNames relies on when given several columns.
func TestSortByNamesIsCumulative(t *testing.T) {
	tbl := &Table{}
	tbl.SetHeader("TEAM", "COUNT")
	tbl.AddRow(NewCellText("blue"), NewCellInt("2", 2))
	tbl.AddRow(NewCellText("red"), NewCellInt("1", 1))
	tbl.AddRow(NewCellText("blue"), NewCellInt("1", 1))
	tbl.AddRow(NewCellText("red"), NewCellInt("2", 2))

	if err := tbl.SortByNames("COUNT", "TEAM"); err != nil {
		t.Fatalf("SortByNames: %v", err)
	}

	var got []string
	for _, row := range tbl.rowOrder {
		got = append(got, tbl.data[row][0].text+tbl.data[row][1].text)
	}

	want := []string{"blue1", "blue2", "red1", "red2"}
	if !slices.Equal(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

// TestSortHandlesNaN guards the float branch: NaN must not panic and must not be
// pulled to the front of the table the way cmp.Compare would order it.
func TestSortHandlesNaN(t *testing.T) {
	tbl := &Table{}
	tbl.SetHeader("USED")
	tbl.AddRow(NewCellFloat("5", 5))
	tbl.AddRow(NewCellFloat("NaN", math.NaN()))
	tbl.AddRow(NewCellFloat("1", 1))

	tbl.sort(tbl.rowOrder, 0, true)

	if got := tbl.data[tbl.rowOrder[0]][0].text; got == "NaN" {
		t.Errorf("NaN sorted to the front of the table, got first row %q", got)
	}
}

func benchmarkTableSort(b *testing.B, rowCount int, sortFn func(*Table, []int, int, bool)) {
	tbl := newSortTestTable(rowCount)
	order := make([]int, len(tbl.rowOrder))

	for b.Loop() {
		copy(order, tbl.rowOrder)
		sortFn(tbl, order, 0, true)
	}
}

func BenchmarkTableSort200(b *testing.B)   { benchmarkTableSort(b, 200, (*Table).sort) }
func BenchmarkTableSort2000(b *testing.B)  { benchmarkTableSort(b, 2000, (*Table).sort) }
func BenchmarkTableSort10000(b *testing.B) { benchmarkTableSort(b, 10000, (*Table).sort) }
