package main

import (
	"log"
	"path/filepath"
	"time"

	"github.com/miskawu/piper/internal/store"
	"github.com/miskawu/piper/internal/tmux"
	"github.com/miskawu/piper/internal/vault"
)

// initVault creates the vault manager, runs migration from old storage,
// and reconciles the stored remote URL with the actual git repo.
func initVault(sessionStore *store.Store) *vault.VaultManager {
	vaultDir := filepath.Join(filepath.Dir(sessionStore.Path()), "vault")
	vm := vault.New(vaultDir)
	if err := vm.Init(); err != nil {
		log.Printf("WARNING: failed to init vault: %v", err)
	}

	// Migrate old epics + ideas into vault
	oldEpicsDir := filepath.Join(filepath.Dir(sessionStore.Path()), "epics")
	oldIdeas := sessionStore.GetIdeas()
	var migrateIdeas []vault.OldIdea
	for _, idea := range oldIdeas {
		old := vault.OldIdea{
			ID:           idea.ID,
			Text:         idea.Text,
			EnrichedDesc: idea.EnrichedDesc,
			CreatedAt:    idea.CreatedAt,
		}
		for _, c := range idea.Context {
			old.Context = append(old.Context, vault.OldIdeaContext{
				SessionName: c.SessionName,
				Kind:        c.Kind,
				Detail:      c.Detail,
				Time:        c.Time,
			})
		}
		migrateIdeas = append(migrateIdeas, old)
	}
	nextID := sessionStore.GetNextIdeaID()
	if err := vm.MigrateIfNeeded(oldEpicsDir, migrateIdeas, nextID); err != nil {
		log.Printf("WARNING: vault migration failed: %v", err)
	}

	// Reconcile stored remote URL with actual git repo
	epicGitCfg := sessionStore.GetEpicGitConfig()
	if epicGitCfg.RemoteURL != "" {
		git := vm.Git()
		if git.RemoteExists(vm.BaseDir(), "origin") {
			git.RemoteSetURL(vm.BaseDir(), "origin", epicGitCfg.RemoteURL)
		} else {
			if err := git.RemoteAdd(vm.BaseDir(), "origin", epicGitCfg.RemoteURL); err != nil {
				log.Printf("WARNING: failed to add vault origin remote: %v", err)
			}
		}
	}

	return vm
}

// cleanupOrphanSessions removes plan and setup tmux sessions
// that no longer have a corresponding epic or project.
func cleanupOrphanSessions(tmuxClient *tmux.Client, vm *vault.VaultManager, sessionStore *store.Store) {
	// Clean orphan plan sessions
	if epics, err := vm.ListEpics(); err == nil {
		epicIDs := make(map[string]bool, len(epics))
		for _, e := range epics {
			epicIDs[e.ID] = true
		}
		if err := tmuxClient.CleanOrphanPlanSessions(epicIDs); err != nil {
			log.Printf("WARNING: failed to clean orphan plan sessions: %v", err)
		}
	}

	// Clean orphan setup sessions
	projects := sessionStore.GetProjects()
	validProjects := make(map[string]bool, len(projects))
	for _, p := range projects {
		validProjects[tmux.SanitizeProjectName(filepath.Base(p))] = true
	}
	if err := tmuxClient.CleanOrphanSetupSessions(validProjects); err != nil {
		log.Printf("WARNING: failed to clean orphan setup sessions: %v", err)
	}

	// Archive stale session metadata: sessions in store but not in tmux
	if tmuxSessions, err := tmuxClient.ListSessions(); err == nil {
		activeNames := make(map[string]bool, len(tmuxSessions))
		for _, ts := range tmuxSessions {
			activeNames[ts.Name] = true
		}
		archived := 0
		for name := range sessionStore.AllSessions() {
			if !activeNames[name] {
				if err := sessionStore.ArchiveSession(name); err != nil {
					log.Printf("WARNING: failed to archive stale session %s: %v", name, err)
				} else {
					archived++
				}
			}
		}
		if archived > 0 {
			log.Printf("archived %d stale session(s) from store", archived)
		}
	}

	// Prune expired recoverable sessions
	if pruned, err := sessionStore.PruneRecoverable(); err != nil {
		log.Printf("WARNING: failed to prune recoverable sessions: %v", err)
	} else if pruned > 0 {
		log.Printf("pruned %d expired recoverable session(s)", pruned)
	}

	// Prune session history older than 90 days
	if pruned, err := sessionStore.PruneHistory(90 * 24 * time.Hour); err != nil {
		log.Printf("WARNING: failed to prune session history: %v", err)
	} else if pruned > 0 {
		log.Printf("pruned %d session history entries older than 90 days", pruned)
	}
}
