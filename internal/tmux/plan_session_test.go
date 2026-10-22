package tmux

import "testing"

func TestParsePlanSessionName_三種前綴(t *testing.T) {
	tests := []struct {
		name     string
		wantEpic string
		wantType string
		wantOK   bool
	}{
		{"pplan-my-epic", "my-epic", "plan", true},
		{"pdisp-my-epic", "my-epic", "dispatch", true},
		{"pretro-my-epic", "my-epic", "retro", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := ParsePlanSessionName(tt.name)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if info.Epic != tt.wantEpic {
				t.Errorf("Epic = %q, want %q", info.Epic, tt.wantEpic)
			}
			if info.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", info.Type, tt.wantType)
			}
			if info.Name != tt.name {
				t.Errorf("Name = %q, want %q", info.Name, tt.name)
			}
		})
	}
}

func TestParsePlanSessionName_非plan前綴回傳false(t *testing.T) {
	tests := []string{
		"ragent-piper-abc123",
		"other-session",
		"plan-epic", // no "p" prefix
		"",
	}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			_, ok := ParsePlanSessionName(name)
			if ok {
				t.Errorf("expected ok=false for %q", name)
			}
		})
	}
}

func TestParsePlanSessionName_前綴後無slug回傳false(t *testing.T) {
	tests := []string{"pplan-", "pdisp-", "pretro-"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			_, ok := ParsePlanSessionName(name)
			if ok {
				t.Errorf("expected ok=false for empty slug %q", name)
			}
		})
	}
}
