package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/miskawu/piper/internal/mousedex"
)

// MousedexEntry represents one collected mouse species.
type MousedexEntry struct {
	SpeciesKey  string `json:"speciesKey"`
	IsSSR       bool   `json:"isSSR"`
	TopIdx      *int   `json:"topIdx"`
	MidIdx      *int   `json:"midIdx"`
	BotIdx      *int   `json:"botIdx"`
	BodyColor   string `json:"bodyColor"`
	Color       string `json:"color"`
	UnlockedAt  string `json:"unlockedAt"`
	SessionName string `json:"sessionName"`
	UnlockCount int    `json:"unlockCount"`
}

// Mousedex is the full collection persisted as mousedex.json.
type Mousedex struct {
	Collected map[string]MousedexEntry `json:"collected"`
}

// MousedexStats is a lightweight summary.
type MousedexStats struct {
	Collected int `json:"collected"`
	Total     int `json:"total"`
}

func (v *VaultManager) mousedexPath() string {
	return filepath.Join(v.baseDir, "mousedex.json")
}

// LoadMousedex reads mousedex.json from vault. Returns empty Mousedex if missing.
func (v *VaultManager) LoadMousedex() (*Mousedex, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.loadMousedexLocked()
}

func (v *VaultManager) loadMousedexLocked() (*Mousedex, error) {
	data, err := os.ReadFile(v.mousedexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Mousedex{Collected: make(map[string]MousedexEntry)}, nil
		}
		return nil, fmt.Errorf("load mousedex: %w", err)
	}
	var mdex Mousedex
	if err := json.Unmarshal(data, &mdex); err != nil {
		return nil, fmt.Errorf("parse mousedex: %w", err)
	}
	if mdex.Collected == nil {
		mdex.Collected = make(map[string]MousedexEntry)
	}
	return &mdex, nil
}

func (v *VaultManager) saveMousedexLocked(mdex *Mousedex) error {
	data, err := json.MarshalIndent(mdex, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mousedex: %w", err)
	}
	if err := os.WriteFile(v.mousedexPath(), data, 0o644); err != nil {
		return fmt.Errorf("write mousedex: %w", err)
	}
	return nil
}

// CollectMouse checks if the mouse species is new, and if so, adds it to the collection.
// Returns true if this was a new species.
func (v *VaultManager) CollectMouse(sessionName string, cfg mousedex.MouseConfig) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	mdex, err := v.loadMousedexLocked()
	if err != nil {
		return false, err
	}

	key := mousedex.SpeciesKey(cfg)
	if existing, exists := mdex.Collected[key]; exists {
		existing.UnlockCount++
		mdex.Collected[key] = existing
		if err := v.saveMousedexLocked(mdex); err != nil {
			return false, err
		}
		return false, nil
	}

	mdex.Collected[key] = MousedexEntry{
		SpeciesKey:  key,
		IsSSR:       cfg.IsSSR,
		TopIdx:      cfg.TopIdx,
		MidIdx:      cfg.MidIdx,
		BotIdx:      cfg.BotIdx,
		BodyColor:   cfg.BodyColor,
		Color:       cfg.Color,
		UnlockedAt:  time.Now().Format("2006-01-02"),
		SessionName: sessionName,
		UnlockCount: 1,
	}

	if err := v.saveMousedexLocked(mdex); err != nil {
		return false, err
	}

	v.autoCommit("mousedex: collect new species", "mousedex.json")
	return true, nil
}

// GetMousedexStats returns a lightweight summary.
func (v *VaultManager) GetMousedexStats() (MousedexStats, error) {
	mdex, err := v.LoadMousedex()
	if err != nil {
		return MousedexStats{}, err
	}
	return MousedexStats{
		Collected: len(mdex.Collected),
		Total:     mousedex.TotalSpecies,
	}, nil
}
