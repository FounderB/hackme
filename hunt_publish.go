package main

import (
	"context"
	"fmt"
	"strings"

	"hackme/internal/hunt"
	"hackme/internal/poolfuzz"
	"hackme/internal/poolsync"
)

func (a *app) publishHuntHarnessForConfig(ctx context.Context, cfg map[string]any) error {
	if cfg == nil || !poolfuzz.IsHuntCampaign(cfg) {
		return nil
	}
	hash := strings.TrimSpace(toString(cfg["harness_hash"]))
	if hash == "" {
		return nil
	}
	root := a.repoRoot()
	cachePath, err := hunt.SafeCacheFile(root, "hunt-harness", hash, "bin")
	if err != nil {
		return err
	}
	if _, err := hunt.SafeStatUnder(root, cachePath); err != nil {
		targetID := strings.TrimSpace(toString(cfg["upstream_target_id"]))
		if targetID == "" {
			return fmt.Errorf("hunt publish: harness binary missing for %s", hash)
		}
		bin, err := hunt.EnsureHarnessBinary(ctx, root, targetID, hash)
		if err != nil {
			return err
		}
		cachePath = bin
	}
	sourceRel := strings.TrimSpace(toString(cfg["hunt_source_rel"]))
	if err := hunt.PublishHarnessFile(ctx, a.db, hash, cachePath, sourceRel); err != nil {
		return err
	}
	cfg["harness_published"] = true
	cfg["harness_fetch_path"] = hunt.HarnessFetchURL(hash)
	return nil
}

func (a *app) syncHuntHarnessToCoordinator(ctx context.Context, cfg map[string]any) error {
	if cfg == nil {
		return nil
	}
	hash := strings.TrimSpace(toString(cfg["harness_hash"]))
	if hash == "" {
		return nil
	}
	coord := poolsync.ResolveCoordinatorURL()
	token := poolsync.AdminToken()
	if coord == "" || token == "" {
		return fmt.Errorf("hunt harness sync: coordinator url/token not configured")
	}
	data, err := hunt.GetHarnessArtifact(ctx, a.db, hash)
	if err != nil {
		return fmt.Errorf("hunt harness sync: local artifact: %w", err)
	}
	if err := poolsync.UploadHuntHarness(ctx, coord, token, hash, data, strings.TrimSpace(toString(cfg["hunt_source_rel"]))); err != nil {
		// Catalog hash is metadata-stable; clang rebuilds can differ byte-for-byte across hosts.
		// "already bound" from the coordinator means that hash is already published and fetchable.
		if poolsync.IsHarnessAlreadyBound(err) {
			return nil
		}
		return err
	}
	return nil
}
