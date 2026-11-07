package compare

import "testing"

// TestTask6ReasonCodesStable защищает машинный контракт причин для потребителей плана.
func TestTask6ReasonCodesStable(t *testing.T) {
	tests := []struct {
		name string
		code ReasonCode
		want string
	}{
		{"PageCountChanged", PageCountChanged, "page_count_changed"},
		{"PageOrderChanged", PageOrderChanged, "page_order_changed"},
		{"PageCorrespondenceAmbiguous", PageCorrespondenceAmbiguous, "page_correspondence_ambiguous"},
		{"NodeCountChanged", NodeCountChanged, "node_count_changed"},
		{"NodeNestingChanged", NodeNestingChanged, "node_nesting_changed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.code); got != tt.want {
				t.Errorf("машинный код = %q, нужен %q", got, tt.want)
			}
		})
	}
}
