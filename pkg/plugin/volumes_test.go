package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestDecodeVolumeTypeFCWithWWIDs(t *testing.T) {
	s := volumes{}
	out := s.decodeVolumeType("FC", v1.VolumeSource{FC: &v1.FCVolumeSource{WWIDs: []string{"wwid-1"}}})
	if out["backing"].text != "wwid-1" {
		t.Errorf("backing = %q, want wwid-1", out["backing"].text)
	}
	out = s.decodeVolumeType("FC", v1.VolumeSource{FC: &v1.FCVolumeSource{}})
	if out["backing"].text != "" {
		t.Errorf("backing = %q, want empty", out["backing"].text)
	}
}
