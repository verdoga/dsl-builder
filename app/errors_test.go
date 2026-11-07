package app

import "testing"

func TestErrorsStableCodeAPI(t *testing.T) {
	const (
		state             = CodeState
		staleReview       = CodeStaleReview
		decision          = CodeDecision
		incompleteWeights = CodeIncompleteWeights
		archiveName       = CodeArchiveName
		destination       = CodeDestination
		targetChanged     = CodeTargetChanged
		unconfirmed       = CodeUnconfirmed
		overwrite         = CodeOverwrite
		recovery          = CodeRecovery
	)
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"CodeState", state, "app.state"},
		{"CodeStaleReview", staleReview, "app.stale_review"},
		{"CodeDecision", decision, "app.decision"},
		{"CodeIncompleteWeights", incompleteWeights, "app.incomplete_weights"},
		{"CodeArchiveName", archiveName, "app.archive_name"},
		{"CodeDestination", destination, "app.destination"},
		{"CodeTargetChanged", targetChanged, "app.target_changed"},
		{"CodeUnconfirmed", unconfirmed, "app.unconfirmed"},
		{"CodeOverwrite", overwrite, "app.overwrite"},
		{"CodeRecovery", recovery, "app.recovery"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("код = %q, нужен %q", tt.got, tt.want)
			}
		})
	}
}
