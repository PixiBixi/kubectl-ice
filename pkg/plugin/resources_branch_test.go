package plugin

import (
	"math"
	"testing"
)

// TestResourceBuildBranchPercentages: memory USED is stored in kB and the
// request/limit in bytes, and %REQ is column 3, %LIMIT column 4.
func TestResourceBuildBranchPercentages(t *testing.T) {
	tests := []struct {
		resourceType       string
		used, req, lim     int64
		wantReq, wantLimit float64
	}{
		{"memory", 500_000, 1_000_000_000, 2_000_000_000, 50, 25},
		{"cpu", 50, 100, 200, 50, 25},
	}

	for _, tt := range tests {
		s := resource{ResourceType: tt.resourceType}
		rows := [][]Cell{{{number: tt.used}, {number: tt.req}, {number: tt.lim}, {}, {}}}
		out, err := s.BuildBranch(BuilderInformation{}, rows)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(out[3].float-tt.wantReq) > 0.01 {
			t.Errorf("%s %%REQ = %v, want %v", tt.resourceType, out[3].float, tt.wantReq)
		}
		if math.Abs(out[4].float-tt.wantLimit) > 0.01 {
			t.Errorf("%s %%LIMIT = %v, want %v", tt.resourceType, out[4].float, tt.wantLimit)
		}
	}
}
