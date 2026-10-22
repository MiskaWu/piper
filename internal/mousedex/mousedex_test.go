package mousedex

import (
	"testing"
)

func intPtr(v int) *int { return &v }

// 跨語言測試：Go 輸出必須與 TypeScript 完全一致
func TestGenerateMouseConfig_CrossLanguage(t *testing.T) {
	tests := []struct {
		seed   string
		hash   uint32
		config MouseConfig
	}{
		{
			seed: "test-session-12345",
			hash: 2079158882,
			config: MouseConfig{
				IsSSR: false, BodyColor: "#E1BEE7", Color: "#9B59B6",
				TopIdx: intPtr(4), MidIdx: nil, BotIdx: nil,
			},
		},
		{
			seed: "session-alpha",
			hash: 3644940700,
			config: MouseConfig{
				IsSSR: false, BodyColor: "#B2EBF2", Color: "#00CED1",
				TopIdx: intPtr(5), MidIdx: intPtr(6), BotIdx: intPtr(4),
			},
		},
		{
			seed: "ragent-piper-a3f1b2c9d0",
			hash: 4050159951,
			config: MouseConfig{
				IsSSR: false, BodyColor: "#F8C8C8", Color: "#FF6B6B",
				TopIdx: intPtr(3), MidIdx: nil, BotIdx: intPtr(5),
			},
		},
		{
			seed: "hello",
			hash: 261238937,
			config: MouseConfig{
				IsSSR: false, BodyColor: "#BBDEFB", Color: "#4D96FF",
				TopIdx: nil, MidIdx: intPtr(1), BotIdx: intPtr(7),
			},
		},
		{
			seed: "ssr-test-42",
			hash: 3777566493,
			config: MouseConfig{
				IsSSR: false, BodyColor: "#D6EAF8", Color: "#A8D8EA",
				TopIdx: intPtr(9), MidIdx: intPtr(0), BotIdx: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.seed, func(t *testing.T) {
			// 驗證 hash
			h := hashString(tt.seed)
			if h != tt.hash {
				t.Errorf("hashString(%q) = %d, want %d", tt.seed, h, tt.hash)
			}

			cfg := GenerateMouseConfig(tt.seed)
			if cfg.IsSSR != tt.config.IsSSR {
				t.Errorf("IsSSR = %v, want %v", cfg.IsSSR, tt.config.IsSSR)
			}
			if cfg.BodyColor != tt.config.BodyColor {
				t.Errorf("BodyColor = %s, want %s", cfg.BodyColor, tt.config.BodyColor)
			}
			if cfg.Color != tt.config.Color {
				t.Errorf("Color = %s, want %s", cfg.Color, tt.config.Color)
			}
			assertIntPtr(t, "TopIdx", cfg.TopIdx, tt.config.TopIdx)
			assertIntPtr(t, "MidIdx", cfg.MidIdx, tt.config.MidIdx)
			assertIntPtr(t, "BotIdx", cfg.BotIdx, tt.config.BotIdx)
		})
	}
}

func assertIntPtr(t *testing.T, name string, got, want *int) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil || want == nil {
		t.Errorf("%s: got %v, want %v", name, got, want)
		return
	}
	if *got != *want {
		t.Errorf("%s: got %d, want %d", name, *got, *want)
	}
}

func TestSpeciesKey(t *testing.T) {
	tests := []struct {
		cfg  MouseConfig
		want string
	}{
		{MouseConfig{IsSSR: true}, "ssr"},
		{MouseConfig{IsSSR: false, TopIdx: nil, MidIdx: nil, BotIdx: nil}, "x-x-x"},
		{MouseConfig{IsSSR: false, TopIdx: intPtr(0), MidIdx: intPtr(3), BotIdx: nil}, "0-3-x"},
		{MouseConfig{IsSSR: false, TopIdx: intPtr(9), MidIdx: intPtr(9), BotIdx: intPtr(9)}, "9-9-9"},
	}
	for _, tt := range tests {
		got := SpeciesKey(tt.cfg)
		if got != tt.want {
			t.Errorf("SpeciesKey(%+v) = %q, want %q", tt.cfg, got, tt.want)
		}
	}
}

func TestEnumerateAllSpeciesKeys(t *testing.T) {
	keys := EnumerateAllSpeciesKeys()
	if len(keys) != TotalSpecies {
		t.Errorf("len = %d, want %d", len(keys), TotalSpecies)
	}
	if keys[0] != "ssr" {
		t.Errorf("first key = %q, want 'ssr'", keys[0])
	}
	// 檢查無重複
	seen := make(map[string]bool)
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate key: %s", k)
		}
		seen[k] = true
	}
}

func TestGenerateMouseConfig_Deterministic(t *testing.T) {
	a := GenerateMouseConfig("same-seed")
	b := GenerateMouseConfig("same-seed")
	if SpeciesKey(a) != SpeciesKey(b) || a.BodyColor != b.BodyColor {
		t.Error("same seed should produce identical config")
	}
}

func TestGenerateMouseConfig_DecorationBounds(t *testing.T) {
	for i := 0; i < 500; i++ {
		cfg := GenerateMouseConfig(string(rune('a'+i)) + "-test")
		checkBound := func(name string, v *int) {
			if v != nil && (*v < 0 || *v >= 10) {
				t.Errorf("seed %d: %s = %d, out of range", i, name, *v)
			}
		}
		checkBound("TopIdx", cfg.TopIdx)
		checkBound("MidIdx", cfg.MidIdx)
		checkBound("BotIdx", cfg.BotIdx)
	}
}
