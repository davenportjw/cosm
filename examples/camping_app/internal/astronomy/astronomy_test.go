package astronomy

import "testing"

func TestObservationRating(t *testing.T) {
	if rating := CalculateObservationRating(10.0, 90, BortleClass1); rating != "OPTIMAL" {
		t.Errorf("expected OPTIMAL, got %s", rating)
	}
	if rating := CalculateObservationRating(80.0, 90, BortleClass1); rating != "POOR" {
		t.Errorf("expected POOR, got %s", rating)
	}
}
