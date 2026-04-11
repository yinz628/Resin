package platform

import "testing"

func TestValidateServiceFilters(t *testing.T) {
	if err := ValidateServiceFilters([]string{"openai", "anthropic", "unsupported"}); err != nil {
		t.Fatalf("ValidateServiceFilters(valid): %v", err)
	}

	if err := ValidateServiceFilters([]string{"bad"}); err == nil {
		t.Fatal("expected error for invalid service filter")
	}
}

func TestMatchServiceFilters(t *testing.T) {
	tests := []struct {
		name        string
		openai      bool
		anthropic   bool
		filters     []string
		wantMatched bool
	}{
		{
			name:        "empty filters always match",
			openai:      false,
			anthropic:   false,
			filters:     []string{},
			wantMatched: true,
		},
		{
			name:        "openai filter match",
			openai:      true,
			anthropic:   false,
			filters:     []string{"openai"},
			wantMatched: true,
		},
		{
			name:        "anthropic filter miss",
			openai:      true,
			anthropic:   false,
			filters:     []string{"anthropic"},
			wantMatched: false,
		},
		{
			name:        "unsupported filter match",
			openai:      false,
			anthropic:   false,
			filters:     []string{"unsupported"},
			wantMatched: true,
		},
		{
			name:        "unsupported filter miss",
			openai:      false,
			anthropic:   true,
			filters:     []string{"unsupported"},
			wantMatched: false,
		},
		{
			name:        "or semantics across filters",
			openai:      false,
			anthropic:   true,
			filters:     []string{"openai", "anthropic"},
			wantMatched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchServiceFilters(tt.openai, tt.anthropic, tt.filters)
			if got != tt.wantMatched {
				t.Fatalf(
					"MatchServiceFilters(openai=%v, anthropic=%v, filters=%v) = %v, want %v",
					tt.openai,
					tt.anthropic,
					tt.filters,
					got,
					tt.wantMatched,
				)
			}
		})
	}
}
