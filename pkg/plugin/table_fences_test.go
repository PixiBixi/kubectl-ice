package plugin

import (
	"math"
	"testing"
)

// fencesBoundarysOld is the implementation getFencesInt and getFencesFloat
// shared before they were split, kept as the reference oracle. It returned any
// and both callers asserted the concrete type unchecked.
func fencesBoundarysOld(orderList []int, columnID int, rows [][]Cell, cellType int) (any, any) {
	var q1Int, q3Int, iqrInt int64
	var q1Float, q3Float, iqrFloat float64

	listLen := len(orderList) + 1
	pos2 := listLen / 2
	pos1 := (pos2 / 2) - 1
	pos3 := pos2 + (pos2 / 2) - 1

	if listLen&1 == 1 {
		rowPos1 := orderList[pos1]
		rowPos2 := orderList[pos1+1]
		rowPos3 := orderList[pos3]
		rowPos4 := orderList[pos3+1]

		t1Cell := rows[rowPos1][columnID]
		t2Cell := rows[rowPos2][columnID]
		t3Cell := rows[rowPos3][columnID]
		t4Cell := rows[rowPos4][columnID]

		switch cellType {
		case 1:
			q1Int = (t1Cell.number + t2Cell.number) / 2
			q3Int = (t3Cell.number + t4Cell.number) / 2
		case 2:
			q1Float = (t1Cell.float + t2Cell.float) / 2
			q3Float = (t3Cell.float + t4Cell.float) / 2
		}
	} else {
		rowPos1 := orderList[pos1]
		rowPos3 := orderList[pos3]
		t1Cell := rows[rowPos1][columnID]
		t3Cell := rows[rowPos3][columnID]

		switch cellType {
		case 1:
			q1Int = t1Cell.number
			q3Int = t3Cell.number
		case 2:
			q1Float = t1Cell.float
			q3Float = t3Cell.float
		}
	}

	if cellType == 1 {
		iqrInt = q3Int - q1Int
		pc := (15 * iqrInt) / 10
		return q3Int + pc, pc - q1Int
	}
	iqrFloat = q3Float - q1Float
	pc := 1.5 * iqrFloat
	return q3Float + pc, pc - q1Float
}

// newFenceTestTable builds rowCount rows with an int column and a float column,
// spread so the quartiles are not all the same value.
func newFenceTestTable(rowCount int) *Table {
	tbl := &Table{}
	tbl.SetHeader("COUNT", "USED")

	for i := range rowCount {
		v := (i*37 + 11) % 500
		tbl.AddRow(
			NewCellInt("", int64(v)),
			NewCellFloat("", float64(v)+0.375),
		)
	}

	return tbl
}

// TestGetFencesMatchesOldImplementation pins the split of getFencesBoundarys
// into two typed functions. The int path truncates on integer division and the
// float path does not, so the two are compared separately rather than merged
// into one generic helper.
func TestGetFencesMatchesOldImplementation(t *testing.T) {
	// pos1 is (listLen/2)/2-1, so short lists index out of range in both the old
	// and new code. Start where the original was usable.
	for rowCount := 7; rowCount <= 200; rowCount++ {
		tbl := newFenceTestTable(rowCount)

		rawUpperInt, rawLowerInt := fencesBoundarysOld(tbl.rowOrder, 0, tbl.data, 1)
		wantUpperInt, ok := rawUpperInt.(int64)
		if !ok {
			t.Fatalf("rows=%d: oracle returned %T for the int upper fence", rowCount, rawUpperInt)
		}
		wantLowerInt, ok := rawLowerInt.(int64)
		if !ok {
			t.Fatalf("rows=%d: oracle returned %T for the int lower fence", rowCount, rawLowerInt)
		}

		gotUpperInt, gotLowerInt := tbl.getFencesInt(tbl.rowOrder, 0, tbl.data)
		if gotUpperInt != wantUpperInt || gotLowerInt != wantLowerInt {
			t.Fatalf("rows=%d int: want (%v, %v), got (%v, %v)",
				rowCount, wantUpperInt, wantLowerInt, gotUpperInt, gotLowerInt)
		}

		rawUpperFloat, rawLowerFloat := fencesBoundarysOld(tbl.rowOrder, 1, tbl.data, 2)
		wantUpperFloat, ok := rawUpperFloat.(float64)
		if !ok {
			t.Fatalf("rows=%d: oracle returned %T for the float upper fence", rowCount, rawUpperFloat)
		}
		wantLowerFloat, ok := rawLowerFloat.(float64)
		if !ok {
			t.Fatalf("rows=%d: oracle returned %T for the float lower fence", rowCount, rawLowerFloat)
		}

		gotUpperFloat, gotLowerFloat := tbl.getFencesFloat(tbl.rowOrder, 1, tbl.data)
		if gotUpperFloat != wantUpperFloat || gotLowerFloat != wantLowerFloat {
			t.Fatalf("rows=%d float: want (%v, %v), got (%v, %v)",
				rowCount, wantUpperFloat, wantLowerFloat, gotUpperFloat, gotLowerFloat)
		}
		if math.IsNaN(gotUpperFloat) || math.IsNaN(gotLowerFloat) {
			t.Fatalf("rows=%d float: produced NaN", rowCount)
		}
	}
}
