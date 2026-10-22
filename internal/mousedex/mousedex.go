package mousedex

import (
	"fmt"
	"math"
)

// 色盤 — 與前端 MouseAvatar.tsx 完全對應
var COLORS = [10]string{
	"#FF6B6B", "#FFB347", "#FFD93D", "#6BCB77", "#4D96FF",
	"#9B59B6", "#FF69B4", "#00CED1", "#FF8C42", "#A8D8EA",
}

var BODY_COLORS = [10]string{
	"#F8C8C8", "#FADCB0", "#FFF2B0", "#C8E6C9", "#BBDEFB",
	"#E1BEE7", "#F8BBD0", "#B2EBF2", "#FFD0A8", "#D6EAF8",
}

const SSR_CHANCE = 0.01

// TotalSpecies is the total number of unique mouse species.
// SSR(1) + normal(11^3 = 1331) = 1332
const TotalSpecies = 1332

// MouseConfig mirrors the frontend MouseConfig.
type MouseConfig struct {
	IsSSR     bool   `json:"isSSR"`
	BodyColor string `json:"bodyColor"`
	Color     string `json:"color"`
	TopIdx    *int   `json:"topIdx"`
	MidIdx    *int   `json:"midIdx"`
	BotIdx    *int   `json:"botIdx"`
}

// SpeciesKey returns a string key identifying the species (ignoring colors).
func SpeciesKey(cfg MouseConfig) string {
	if cfg.IsSSR {
		return "ssr"
	}
	f := func(v *int) string {
		if v == nil {
			return "x"
		}
		return fmt.Sprintf("%d", *v)
	}
	return fmt.Sprintf("%s-%s-%s", f(cfg.TopIdx), f(cfg.MidIdx), f(cfg.BotIdx))
}

// hashString implements DJB2 hash, matching the frontend.
func hashString(s string) uint32 {
	hash := int32(5381)
	for i := 0; i < len(s); i++ {
		// ((hash << 5) + hash + charCode) | 0
		hash = ((hash << 5) + hash + int32(s[i]))
	}
	return uint32(hash)
}

// mulberry32 returns a seeded PRNG matching the frontend mulberry32.
func mulberry32(seed uint32) func() float64 {
	s := int32(seed)
	return func() float64 {
		s = s + int32(0x6d2b79f5)
		// Math.imul(a, b) in JS = int32 multiplication
		t := imul(int32(uint32(s)^(uint32(s)>>15)), 1|s)
		t = (t + imul(int32(uint32(t)^(uint32(t)>>7)), 61|t)) ^ t
		return float64(uint32(t)^(uint32(t)>>14)) / 4294967296.0
	}
}

// imul emulates JavaScript's Math.imul (32-bit integer multiply).
func imul(a, b int32) int32 {
	return int32(int64(a) * int64(b))
}

// GenerateMouseConfig generates a mouse config from a session name seed.
// This must produce identical output to the frontend generateMouseConfig.
func GenerateMouseConfig(seed string) MouseConfig {
	h := hashString(seed)
	rand := mulberry32(h)

	isSSR := rand() < SSR_CHANCE
	bodyIdx := int(math.Floor(rand() * float64(len(BODY_COLORS))))
	bodyColor := BODY_COLORS[bodyIdx]
	color := COLORS[bodyIdx]

	var topIdx, midIdx, botIdx *int
	if !isSSR {
		topIdx = randDeco(rand, 10)
		midIdx = randDeco(rand, 10)
		botIdx = randDeco(rand, 10)
	}

	return MouseConfig{
		IsSSR:     isSSR,
		BodyColor: bodyColor,
		Color:     color,
		TopIdx:    topIdx,
		MidIdx:    midIdx,
		BotIdx:    botIdx,
	}
}

func randDeco(rand func() float64, n int) *int {
	if rand() < 0.5 {
		return nil
	}
	v := int(math.Floor(rand() * float64(n)))
	return &v
}

// EnumerateAllSpeciesKeys returns all 1332 species keys in a deterministic order.
func EnumerateAllSpeciesKeys() []string {
	keys := make([]string, 0, TotalSpecies)
	keys = append(keys, "ssr")
	f := func(v int) string {
		if v < 0 {
			return "x"
		}
		return fmt.Sprintf("%d", v)
	}
	for t := -1; t <= 9; t++ {
		for m := -1; m <= 9; m++ {
			for b := -1; b <= 9; b++ {
				keys = append(keys, fmt.Sprintf("%s-%s-%s", f(t), f(m), f(b)))
			}
		}
	}
	return keys
}
