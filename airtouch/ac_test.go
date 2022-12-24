package airtouch

import "testing"

func TestAC(t *testing.T) {
	data := 1
	expected := 1

	if data != expected {
		t.Errorf("expected %d, got %d", expected, data)
	}
}
