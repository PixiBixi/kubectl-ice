package plugin

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
)

// sets the maximum number of spaces allowed in a column, spaces are clipped to this number
const maxLineLength = 80

type headerRow struct {
	columnLength int
	columnType   int // 0:string, 1:int
	hidden       bool
	sort         int // 0:no-sort, 1:sort-forward, 2:sort-backward
	title        string
}

type Cell struct {
	text   string
	number int64
	float  float64
	typ    int // 0=string, 1=int64, 2=float64, 3=placeholder
	phRef  int // placeholder reference id, used to track the row thats used as a placeholder
	indent int // the number of indents required in the output
	colour [2]int
}

type Table struct {
	currentRow    int
	headCount     int
	columnOrder   []int
	rowOrder      []int
	head          []headerRow
	data          [][]Cell
	hideRow       []bool
	placeHolder   map[int][]Cell
	placeHolderID int
	ColourOutput  int
	CustomColours [][2]int
}

// SetHeader sets the header row to the specified array of strings
// headerRow is always reinitilized to empty before headers are added
func (t *Table) SetHeader(headItem ...string) {

	t.head = make([]headerRow, len(headItem))

	if len(t.columnOrder) == 0 {
		t.columnOrder = []int{}
	}

	for i := range headItem {
		tmpHead := headerRow{}
		tmpHead.title = headItem[i]
		tmpHead.columnLength = len(headItem[i]) + 2
		tmpHead.sort = 0

		t.head[i] = tmpHead

		t.columnOrder = append(t.columnOrder, i)
	}

	t.headCount = len(headItem)
}

// AddRow Adds a new row to the end of the table, accepts an array of strings
func (t *Table) AddRow(row ...Cell) {
	log := logger{location: "Table:AddRow"}
	log.Debug("Start")

	if t.headCount > len(row) {
		panic("not enough columns in provided row")
	}

	for i := 0; i < t.headCount; i++ {
		strLen := utf8.RuneCountInString(row[i].text)
		if row[i].indent > 0 {
			strLen += t.indentLen(row[i].indent)
		}
		if strLen >= t.head[i].columnLength {
			if (strLen + 2) > maxLineLength {
				t.head[i].columnLength = maxLineLength
			} else {
				t.head[i].columnLength = strLen + 2
			}
		}

		switch row[i].typ {
		case 2:
			t.head[i].columnType = 2
		case 1:
			t.head[i].columnType = 1
		}
	}

	t.data = append(t.data, row)                  // add data to row
	t.rowOrder = append(t.rowOrder, t.currentRow) // add row number to end of sort list
	t.hideRow = append(t.hideRow, false)
	t.currentRow += 1

}

// Order changes the order of columns displayed in the table, specifying a subset of the column
// numbers will place those at the front in the order specified all other columns remain untouched
func (t *Table) Order(items ...int) {
	// rather then reordering all columns we have an order array that we can loop through
	// order contains the actual column number to use next
	orderedList := []int{}

	for i := 0; i < len(t.columnOrder); i++ {
		found := false
		for c := range items {
			if items[c] == t.columnOrder[i] {
				found = true
			}
		}
		if !found {
			orderedList = append(orderedList, t.columnOrder[i])
		}
	}
	orderedList = append(items, orderedList...)

	t.columnOrder = orderedList

}

// HideColumn select the column number to hide, columns numbers are the unsorted column number
func (t *Table) HideColumn(columnNumber int) {
	log := logger{location: "Table:HideColumn"}
	log.Debug("Start")

	log.Debug("columnNumber =", columnNumber)
	log.Debug("len(t.head) =", len(t.head))
	if len(t.head) > columnNumber {
		log.Debug("hide =", t.head[columnNumber].title)
		t.head[columnNumber].hidden = true
	} else {
		panic(fmt.Sprintln("invalid column number", columnNumber))
	}
}

// HideTheseColumns hides the column number to hide, columns numbers are the unsorted column number
func (t *Table) HideOnlyNamedColumns(columnName []string) error {
	var found bool
	var validNames []string

	log := logger{location: "Table:HideOnlyNamedColumns"}
	log.Debug("Start")

	log.Debug("len(columnName) =", len(columnName))
	// // unhide every column
	for i := range t.head {
		t.head[i].hidden = true
		validNames = append(validNames, t.head[i].title)
	}

	// hide only the listed columns
	for _, c := range columnName {
		found = false
		for i, h := range t.head {
			if c == h.title {
				log.Debug("hide =", h.title)
				t.head[i].hidden = false
				found = true
			}
		}
		if !found {
			// 	t.head[i].hidden = true
			return fmt.Errorf("error: invalid column \"%s\" current valid column names are as following %s", c, validNames)
		}
	}
	return nil
}

// Fprint outputs the table to w, taking the column order and visibility into account
func (t *Table) Fprint(w io.Writer) {
	var cellcolour [2]int
	var withColour bool
	var visibleColumns int

	var headLineBuf strings.Builder
	colourArray := make([][2]int, t.headCount)

	switch t.ColourOutput {
	case COLOUR_NONE:
		withColour = false
	case COLOUR_CUSTOMMIX:
		fallthrough
	case COLOUR_CUSTOM:
		withColour = true
		maxColours := len(t.CustomColours)
		for i := 0; i < t.headCount; i++ {
			colourCode := int(math.Mod(float64(i), float64(maxColours)))
			colourArray[i][0] = t.CustomColours[colourCode][0] // colour
			colourArray[i][1] = t.CustomColours[colourCode][1] // colour modifier
		}
	default:
		withColour = true

		// generate the colour numbers for the default colour wheel, the colour set is repeated if there are more heades than colours
		maxColours := 14
		modFlip := 0

		for i := 0; i < t.headCount; i++ {
			colourCode := int(math.Mod(float64(i), float64(maxColours)))
			if colourCode < 6 {
				// we start at 31 and increase for 6 colours this allow us to excclude black and light gray
				colourArray[i][0] = colourCode + 31
			} else {
				// the second set covers the dark variations of the colours
				colourArray[i][0] = colourCode + 84
			}

			// we flip the text to bold after every colour run
			colourArray[i][1] = modFlip

			if colourCode >= maxColours-1 {
				modFlip += 1
				if modFlip > 1 {
					modFlip = 0
				}
			}
		}
	}

	// loop through all headers and make a single line properly spaced
	for col := 0; col < t.headCount; col++ {
		// columnOrder contains the actual column number to use next
		idx := t.columnOrder[col]
		if t.head[idx].hidden {
			continue
		}

		cellcolour = colourArray[visibleColumns]
		visibleColumns += 1

		word := t.head[idx].title
		runelen := utf8.RuneCountInString(word)

		if len(word) == 0 {
			word = "-"
		}

		if t.ColourOutput != COLOUR_NONE && t.ColourOutput != COLOUR_ERRORS {
			word = fmt.Sprintf("\033[%d;%dm%s%s", cellcolour[1], cellcolour[0], word, colourEnd)
		}
		pad := strings.Repeat(" ", t.head[idx].columnLength-runelen)

		headLineBuf.WriteString(word)
		headLineBuf.WriteString(pad)
	}
	// print the header in one long line
	fmt.Fprintln(w, strings.TrimRight(headLineBuf.String(), " "))

	// loop through each row
	var lineBuf strings.Builder
	for r := 0; r < len(t.data); r++ {
		var row []Cell

		visibleColumns = 0
		lineBuf.Reset()
		excludeRow := false
		rowNum := t.rowOrder[r]

		if t.hideRow[rowNum] {
			continue
		}

		if t.data[rowNum][0].typ == 3 {
			row = t.placeHolder[t.data[rowNum][0].phRef]
		} else {
			row = t.data[rowNum]
		}
		// now loop through each column in the currentl selected row
		for col := 0; col < t.headCount; col++ {
			idx := t.columnOrder[col]
			cell := row[idx]

			if t.head[idx].hidden {
				// dont process the row if its hidden
				continue
			}

			if withColour { // if colour wanted
				cellcolour = colourArray[visibleColumns] // set colour from wheel as default colour
				switch t.ColourOutput {
				case COLOUR_ERRORS:
					// override if we should only show error colours
					if cell.colour[0] != colourNone {
						cellcolour = cell.colour
					} else {
						cellcolour[0] = -1
					}
				case COLOUR_CUSTOMMIX:
					fallthrough
				case COLOUR_MIX:
					// semantic cells override the wheel; plain cells get no color
					// (headers keep the wheel for column identification)
					if cell.colour[0] > 0 {
						cellcolour = cell.colour
					} else {
						cellcolour[0] = -1
					}
				}
			}

			visibleColumns += 1

			if len(cell.text) == 0 {
				cell.text = "-"
			}

			origtxt := t.indentText(cell.indent, cell.text)
			celltxt := origtxt
			spaceCount := t.head[idx].columnLength - utf8.RuneCountInString(origtxt)
			if spaceCount <= 0 {
				spaceCount = maxLineLength
			}
			pad := strings.Repeat(" ", spaceCount)

			// colour output has been set and the cell has data
			if withColour && cellcolour[0] != colourNone {
				// so we add the colour codes and modifier
				celltxt = fmt.Sprintf("\033[%d;%dm%s%s", cellcolour[1], cellcolour[0], origtxt, colourEnd)
			}

			lineBuf.WriteString(celltxt)
			lineBuf.WriteString(pad)
		}
		if !excludeRow {
			fmt.Fprintln(w, strings.TrimRight(lineBuf.String(), " "))
		}
	}
}

// Print outputs the table to stdout
func (t *Table) Print() { t.Fprint(os.Stdout) }

// Sprint returns the table as a string
func (t *Table) Sprint() string {
	var sb strings.Builder
	t.Fprint(&sb)
	return sb.String()
}

// FprintJson outputs the table to w as json
func (t *Table) FprintJson(w io.Writer) {
	fmt.Fprintln(w, "{\"data\":[")
	var lineBuf strings.Builder
	var quoteBuf []byte
	for rowNum := 0; rowNum < len(t.data); rowNum++ {
		lineBuf.Reset()
		lineBuf.WriteString("{")
		row := t.data[rowNum]
		for col := 0; col < t.headCount; col++ {
			quoteBuf = appendQuoted(quoteBuf, t.head[col].title)
			lineBuf.Write(quoteBuf)
			lineBuf.WriteString(": ")
			quoteBuf = appendQuoted(quoteBuf, row[col].text)
			lineBuf.Write(quoteBuf)
			if col+1 < t.headCount {
				lineBuf.WriteString(", ")
			}
		}
		lineBuf.WriteString("}")
		if rowNum+1 < len(t.data) {
			lineBuf.WriteString(", ")
		}
		fmt.Fprintln(w, lineBuf.String())
	}
	fmt.Fprintln(w, "]}")
}

// PrintJson outputs the table to stdout as json
func (t *Table) PrintJson() { t.FprintJson(os.Stdout) }

// SprintJson returns the table as a json string
func (t *Table) SprintJson() string {
	var sb strings.Builder
	t.FprintJson(&sb)
	return sb.String()
}

// FprintYaml outputs the table to w as yaml
func (t *Table) FprintYaml(w io.Writer) {
	fmt.Fprintln(w, "data:")
	var lineBuf strings.Builder
	var quoteBuf []byte
	for rowNum := 0; rowNum < len(t.data); rowNum++ {
		lineBuf.Reset()
		sep := "-"
		row := t.data[rowNum]
		for col := 0; col < t.headCount; col++ {
			lineBuf.WriteString(sep)
			lineBuf.WriteString(" ")
			lineBuf.WriteString(t.head[col].title)
			lineBuf.WriteString(": ")
			quoteBuf = appendQuoted(quoteBuf, row[col].text)
			lineBuf.Write(quoteBuf)
			lineBuf.WriteString("\n")
			sep = " "
		}
		fmt.Fprint(w, lineBuf.String())
	}
}

// PrintYaml outputs the table to stdout as yaml
func (t *Table) PrintYaml() { t.FprintYaml(os.Stdout) }

// SprintYaml returns the table as a yaml string
func (t *Table) SprintYaml() string {
	var sb strings.Builder
	t.FprintYaml(&sb)
	return sb.String()
}

// FprintList outputs the table to w as key: value pairs
func (t *Table) FprintList(w io.Writer) {
	for rowNum := 0; rowNum < len(t.data); rowNum++ {
		row := t.data[rowNum]
		for col := 0; col < t.headCount; col++ {
			word := row[col].text
			if len(word) == 0 {
				word = ""
			}
			fmt.Fprintln(w, t.head[col].title+":", word)
		}
	}
}

// PrintList outputs the table to stdout as key: value pairs
func (t *Table) PrintList() { t.FprintList(os.Stdout) }

// SprintList returns the table as a list string
func (t *Table) SprintList() string {
	var sb strings.Builder
	t.FprintList(&sb)
	return sb.String()
}

// FprintCsv outputs the table to w as CSV
func (t *Table) FprintCsv(w io.Writer) {
	if len(t.data) == 0 {
		return
	}

	var lineBuf strings.Builder
	for col := 0; col < t.headCount; col++ {
		writeCsvField(&lineBuf, t.head[col].title)
		if col+1 < t.headCount {
			lineBuf.WriteString(", ")
		}
	}
	fmt.Fprintln(w, lineBuf.String())

	for rowNum := 0; rowNum < len(t.data); rowNum++ {
		lineBuf.Reset()
		row := t.data[rowNum]
		for col := 0; col < t.headCount; col++ {
			writeCsvField(&lineBuf, row[col].text)
			if col+1 < t.headCount {
				lineBuf.WriteString(", ")
			}
		}
		fmt.Fprintln(w, lineBuf.String())
	}
}

// appendQuoted writes value into dst as a quoted json string literal, escaping
// the quotes and control characters that used to be emitted raw and produced
// invalid json. Reuses dst so the writers stay allocation free per cell.
func appendQuoted(dst []byte, value string) []byte {
	// invalid utf-8 is reported as an error but still written back as the
	// replacement character, and these writers have no way to return it
	out, _ := jsontext.AppendQuote(dst[:0], value)
	return out
}

// writeCsvField writes value as an RFC 4180 quoted field, doubling any quote it
// contains. An unescaped quote used to end the field early for every parser.
func writeCsvField(dst *strings.Builder, value string) {
	dst.WriteString("\"")
	for {
		idx := strings.IndexByte(value, '"')
		if idx < 0 {
			break
		}
		dst.WriteString(value[:idx])
		dst.WriteString("\"\"")
		value = value[idx+1:]
	}
	dst.WriteString(value)
	dst.WriteString("\"")
}

// PrintCsv outputs the table to stdout as CSV
func (t *Table) PrintCsv() { t.FprintCsv(os.Stdout) }

// SprintCsv returns the table as a CSV string
func (t *Table) SprintCsv() string {
	var sb strings.Builder
	t.FprintCsv(&sb)
	return sb.String()
}

// sort Sorts via the column number, uses the full column count including hidden columns
//
//	function can be run multiple times and is cumalitive
func (t *Table) sort(list []int, columnNumber int, ascending bool) {
	// rather then reordering all rows we have an order array that we can loop through
	// sort contains the actual row number to use next

	// must stay stable: SortByNames calls us once per column and relies on earlier
	// columns keeping their relative order. Do NOT swap for slices.SortFunc.
	slices.SortStableFunc(list, func(a, b int) int {
		cellA := t.data[a][columnNumber]
		cellB := t.data[b][columnNumber]

		var order int
		switch cellA.typ {
		case 0:
			order = strings.Compare(cellA.text, cellB.text)
		case 1:
			switch {
			case cellA.number < cellB.number:
				order = -1
			case cellA.number > cellB.number:
				order = 1
			}
		case 2:
			// explicit compares, not cmp.Compare: cmp orders NaN below every value, which
			// would reshuffle rows the old sort left untouched. Metrics can divide by zero.
			switch {
			case cellA.float < cellB.float:
				order = -1
			case cellA.float > cellB.float:
				order = 1
			}
		}
		// typ 3 (placeholder) falls through as equal, as it did before

		if !ascending {
			return -order
		}
		return order
	})
}

// SortByNames given a , separated list of names match them to actual headers and sort each one in order
// by default sorts in ascending to revers use ! in front of the header name
// returns error on fail and nil otherwise
func (t *Table) SortByNames(name ...string) error {
	columnIds := make([]int, len(name))
	columnFound := make([]bool, len(name))
	columnDescend := make([]bool, len(name))

	if len(name) == 0 {
		return nil
	}

	// scan and match all column names against headers
	for i := range name {
		rawName := strings.TrimSpace(name[i])
		if len(rawName) == 0 {
			continue
		}

		// do we need to sort descending
		if strings.HasPrefix(rawName, "!") {
			if len(rawName) == 1 {
				continue
			}
			// remove ! from start of word
			rawName = rawName[1:]
			columnDescend[i] = true
		}

		// loop all header looking for a match
		for c := 0; c < len(t.head); c++ {
			if rawName != t.head[c].title {
				// skip if we dont have a name match
				continue
			}
			// save the matched column id to our array
			columnIds[i] = c
			columnFound[i] = true
		}
	}

	// sort each one in order
	for i := range columnIds {
		if columnFound[i] {
			// sort function uses ascending true and descending false so we
			// invert descending fLAG to create our ascending flag
			ascend := !columnDescend[i]
			t.sort(t.rowOrder, columnIds[i], ascend)
		}
	}

	return nil
}

// strMatch run a pattten match, accepts * and ?
func strMatch(str string, pattern string) bool {
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

func NewCellEmpty() Cell {
	return Cell{
		typ:    -1,
		colour: [2]int{colourNone, 0},
	}
}

// NewCellText quick wrapper to return a cell object containing the given string
func NewCellText(text string) Cell {

	temp := strings.ReplaceAll(text, "\r", "\\r")
	temp = strings.ReplaceAll(temp, "\f", "\\f")
	temp = strings.ReplaceAll(temp, "\n", "\\n")
	temp = strings.ReplaceAll(temp, "\t", "\\t")

	return Cell{
		text:   temp,
		colour: [2]int{colourNone, 0},
	}
}

// NewCellTextIndent creates a text cell with an indentation indicator, this dosen't actually indent the cell it just
//
//	tells table.go Print to indent it for us
func NewCellTextIndent(text string, indentLevel int) Cell {

	temp := strings.ReplaceAll(text, "\r", "\\r")
	temp = strings.ReplaceAll(temp, "\f", "\\f")
	temp = strings.ReplaceAll(temp, "\n", "\\n")
	temp = strings.ReplaceAll(temp, "\t", "\\t")

	return Cell{
		text:   temp,
		indent: indentLevel,
		colour: [2]int{colourNone, 0},
	}
}

// NewCellInt quick wrapper to return a cell object containing the given string and int
func NewCellInt(text string, value int64) Cell {
	return Cell{
		text:   text,
		number: value,
		typ:    1,
		colour: [2]int{colourNone, 0},
	}
}

// NewCellFloat quick wrapper to return a cell object containing the given string float
func NewCellFloat(text string, value float64) Cell {
	return Cell{
		text:   text,
		float:  value,
		typ:    2,
		colour: [2]int{colourNone, 0},
	}
}

// NewCellColourText quick wrapper to return a cell object containing the given string and the colour to be used
func NewCellColourText(colour [2]int, text string) Cell {

	temp := strings.ReplaceAll(text, "\r", "\\r")
	temp = strings.ReplaceAll(temp, "\f", "\\f")
	temp = strings.ReplaceAll(temp, "\n", "\\n")
	temp = strings.ReplaceAll(temp, "\t", "\\t")

	return Cell{
		text:   temp,
		colour: colour,
	}
}

// NewCellColorInt quick wrapper to return a cell object containing the given colour, string and int
func NewCellColourInt(colour [2]int, text string, value int64) Cell {
	return Cell{
		text:   text,
		number: value,
		typ:    1,
		colour: colour,
	}
}

// NewCellFloat quick wrapper to return a cell object containing the given colour, string and float
func NewCellColourFloat(colour [2]int, text string, value float64) Cell {
	return Cell{
		text:   text,
		float:  value,
		typ:    2,
		colour: colour,
	}
}

// ListOutOfRange when given a columnID to work with it will calculate a range and
// returns a list of rows with values outside that range
func (t *Table) ListOutOfRange(columnID int) ([]int, error) {
	var upperFenceInt, lowerFenceInt int64
	var upperFenceFloat, lowerFenceFloat float64

	cellType := t.data[0][columnID].typ

	if cellType == 0 {
		return []int{}, errors.New("error: unable to creaate a range with strings")
	}

	orderList := make([]int, len(t.data))

	visibleRows := 0
	for i, v := range t.data {
		cell := v[columnID]
		orderList[i] = i
		if cellType != cell.typ {
			return []int{}, errors.New("error: table cell types dont match")
		}
		if !t.hideRow[i] {
			visibleRows += 1
		}
	}

	if visibleRows <= 4 {
		return []int{}, errors.New("error: not enough visible rows to calculate useful range")
	}

	t.sort(orderList, columnID, true)
	if cellType == 1 {
		upperFenceInt, lowerFenceInt = t.getFencesInt(orderList, columnID, t.data)
	} else {
		upperFenceFloat, lowerFenceFloat = t.getFencesFloat(orderList, columnID, t.data)
	}

	out := []int{}

	for k, v := range t.data {
		keep := false
		cell := v[columnID]
		if cellType == 1 {
			if upperFenceInt < cell.number {
				keep = true
			}
			if lowerFenceInt > cell.number {
				keep = true
			}
		} else {
			if upperFenceFloat < cell.float {
				keep = true
			}
			if lowerFenceFloat > cell.float {
				keep = true
			}
		}
		if !keep {
			out = append(out, k)
		}
	}

	return out, nil
}

// GetRows does what it says on the tin
func (t *Table) GetRows() [][]Cell {
	return t.data
}

// HideRows just sets the hide row flag, used by the print function to exclude the row from the output
func (t *Table) HideRows(rowID []int) {
	for _, v := range rowID {
		t.hideRow[v] = true
	}
}

// fenceQuartileRows returns the row numbers holding the first and third
// quartile of orderList. When the middle falls between two rows both are
// returned and the caller averages them, which is what the previous single
// function did through a cellType switch.
func fenceQuartileRows(orderList []int) (q1Rows, q3Rows []int) {
	// find the middle point in the list so we can split the list into 3
	listLen := len(orderList) + 1
	pos2 := listLen / 2
	pos1 := (pos2 / 2) - 1
	pos3 := pos2 + (pos2 / 2) - 1

	if listLen&1 == 1 {
		// the middle is held by 2 items, so we grab 2 points for the 1st third
		// and 2 points for the 3rd third
		return []int{orderList[pos1], orderList[pos1+1]}, []int{orderList[pos3], orderList[pos3+1]}
	}

	// a single middle point per third
	return []int{orderList[pos1]}, []int{orderList[pos3]}
}

// getFencesInt given the current order and a list of rows calculate the upper and lower boundy exclusion limit for the selected columnID
func (t *Table) getFencesInt(orderList []int, columnID int, rows [][]Cell) (int64, int64) {
	q1Rows, q3Rows := fenceQuartileRows(orderList)

	var q1, q3 int64
	for _, row := range q1Rows {
		q1 += rows[row][columnID].number
	}
	q1 /= int64(len(q1Rows))
	for _, row := range q3Rows {
		q3 += rows[row][columnID].number
	}
	q3 /= int64(len(q3Rows))

	// 1.5 times the distance between the 1st and 3rd third gives the fences,
	// everything outside them is excluded. Integer division truncates here, as
	// it did before.
	pc := (15 * (q3 - q1)) / 10
	return q3 + pc, pc - q1
}

// getFencesFloat given the current order and a list of rows calculate the upper and lower boundy exclusion limit for the selected columnID
func (t *Table) getFencesFloat(orderList []int, columnID int, rows [][]Cell) (float64, float64) {
	q1Rows, q3Rows := fenceQuartileRows(orderList)

	var q1, q3 float64
	for _, row := range q1Rows {
		q1 += rows[row][columnID].float
	}
	q1 /= float64(len(q1Rows))
	for _, row := range q3Rows {
		q3 += rows[row][columnID].float
	}
	q3 /= float64(len(q3Rows))

	pc := 1.5 * (q3 - q1)
	return q3 + pc, pc - q1
}

// AddPlaceHolderRow - Adds an updatable row to the table, returns an update id that can be used with UpdatePlaceHolderRow
func (t *Table) AddPlaceHolderRow() int {
	var cellRow []Cell

	id := t.placeHolderID
	t.placeHolderID++

	for i := 0; i < t.headCount; i++ {
		cellRow = append(cellRow, Cell{
			// text:  "PH" + fmt.Sprint(id),
			typ:   3,
			phRef: id,
		})
	}

	t.AddRow(cellRow...)
	if len(t.placeHolder) == 0 {
		t.placeHolder = make(map[int][]Cell, 1)
	}
	t.placeHolder[id] = cellRow

	return id
}

// UpdatePlaceHolderRow - updates the given placeholder at id with the contents of cellList
func (t *Table) UpdatePlaceHolderRow(id int, cellList []Cell) {

	for i := 0; i < t.headCount; i++ {
		strLen := len([]rune(cellList[i].text))
		if cellList[i].indent > 0 {
			strLen += t.indentLen(cellList[i].indent)
		}
		if strLen >= t.head[i].columnLength {
			if (strLen + 2) > maxLineLength {
				t.head[i].columnLength = maxLineLength
			} else {
				t.head[i].columnLength = strLen + 2
			}
		}
	}
	t.placeHolder[id] = cellList
}

// HidePlaceHolderRow matches the placeholder id to an actual row number and calls HideRows to hide the row
func (t *Table) HidePlaceHolderRow(id int) {
	for r := 0; r < len(t.data); r++ {
		rowNum := t.rowOrder[r]

		if t.data[rowNum][0].phRef == id {
			t.HideRows([]int{r})
		}
	}
}

// indentText indents the text to the specified level adds └─ for every level above 0
func (t *Table) indentText(level int, data string) string {
	var indent string

	if level == 0 {
		return data
	}

	if level == 1 {
		indent = "└─"
	}

	if level >= 2 {
		indent = strings.Repeat(" ", level) + "└─"
	}

	return fmt.Sprint(indent, data)
}

// indentLen returns the number of characters that would be indented at the provided level
func (t *Table) indentLen(level int) int {
	var indent int

	if level == 0 {
		return 0
	}

	if level == 1 {
		indent = 2
	}

	if level >= 2 {
		indent = level + 2
	}

	return indent
}
