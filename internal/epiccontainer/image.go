package epiccontainer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/miskawu/piper/internal/devcontainer"
)

var (
	imageVersionCache    string
	imageVersionCacheAt  time.Time
	imageVersionCacheSet bool
	imageVersionCacheMu  sync.Mutex
)

const imageVersionCacheTTL = 5 * time.Minute

func invalidateImageVersionCache() {
	imageVersionCacheMu.Lock()
	defer imageVersionCacheMu.Unlock()
	imageVersionCache = ""
	imageVersionCacheAt = time.Time{}
	imageVersionCacheSet = false
}

// EnsureImage builds the epic container image if it doesn't exist.
// templateFS should be devcontainer.TemplateFS() which embeds base.Dockerfile + scripts.
func EnsureImage(templateFS fs.FS) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "image", "inspect", ImageName).Run(); err == nil {
		return nil
	}
	return buildImage(templateFS, "")
}

// RebuildImage rebuilds the epic container image, passing CLAUDE_CODE_VERSION
// as a build-arg so the final Dockerfile layer (npm install) cache-busts while
// earlier layers hit cache. Pass empty targetVersion to use the Dockerfile default.
// On success, invalidates the image version cache.
func RebuildImage(templateFS fs.FS, targetVersion string) error {
	if err := buildImage(templateFS, targetVersion); err != nil {
		return err
	}
	invalidateImageVersionCache()
	return nil
}

// buildImage extracts embedded templates to a temp build context and runs `docker build`.
// If targetVersion is non-empty, passes --build-arg CLAUDE_CODE_VERSION=<version>.
func buildImage(templateFS fs.FS, targetVersion string) error {
	buildCtx, err := os.MkdirTemp("", "piper-epic-build-*")
	if err != nil {
		return fmt.Errorf("create build context: %w", err)
	}
	defer os.RemoveAll(buildCtx)

	filesToExtract := []string{
		"templates/base.Dockerfile",
		"templates/init-firewall.sh",
		"templates/git-ssh-helper.sh",
		"templates/piper-hook.mjs",
		"templates/piper-statusline.mjs",
		"templates/piper-hooks.json",
	}
	for _, name := range filesToExtract {
		data, err := fs.ReadFile(templateFS, name)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", name, err)
		}
		dest := filepath.Join(buildCtx, filepath.Base(name))
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	args := []string{"build", "-t", ImageName, "-f", filepath.Join(buildCtx, "base.Dockerfile")}
	if targetVersion != "" {
		args = append(args, "--build-arg", "CLAUDE_CODE_VERSION="+targetVersion)
	}
	args = append(args, buildCtx)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	return nil
}

// GetImageClaudeVersion returns the Claude Code version bundled in the epic image.
// Returns empty string (no error) if the image does not exist.
// Results are cached for 5 minutes; RebuildImage invalidates the cache on success.
func GetImageClaudeVersion(ctx context.Context) (string, error) {
	imageVersionCacheMu.Lock()
	if imageVersionCacheSet && time.Since(imageVersionCacheAt) < imageVersionCacheTTL {
		v := imageVersionCache
		imageVersionCacheMu.Unlock()
		return v, nil
	}
	imageVersionCacheMu.Unlock()

	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(runCtx, "docker", "run", "--rm", "--entrypoint", "claude", ImageName, "--version").CombinedOutput()
	version := ""
	if err != nil {
		if !strings.Contains(string(out), "No such image") && !strings.Contains(string(out), "Unable to find image") {
			return "", fmt.Errorf("run claude --version in image: %w", err)
		}
	} else {
		version = devcontainer.ParseClaudeVersion(strings.TrimSpace(string(out)))
		if version == "" {
			return "", fmt.Errorf("cannot parse version from %q", string(out))
		}
	}

	imageVersionCacheMu.Lock()
	imageVersionCache = version
	imageVersionCacheAt = time.Now()
	imageVersionCacheSet = true
	imageVersionCacheMu.Unlock()
	return version, nil
}
