package migrator

import "testing"

func TestShouldSyncTaskConfig(t *testing.T) {
	tests := []struct {
		name          string
		validateOnly  bool
		skipRequested bool
		want          bool
	}{
		{name: "normal migration", want: true},
		{name: "explicit skip", skipRequested: true, want: false},
		{name: "validate only is read only", validateOnly: true, want: false},
		{name: "validate only explicit skip", validateOnly: true, skipRequested: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncTaskConfig(tt.validateOnly, tt.skipRequested); got != tt.want {
				t.Fatalf("shouldSyncTaskConfig(%t, %t) = %t, want %t", tt.validateOnly, tt.skipRequested, got, tt.want)
			}
		})
	}
}
