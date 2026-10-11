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
	// Publish afl-cc showmap harness when built at campaign create.
	if covHash := strings.TrimSpace(toString(cfg["coverage_harness_hash"])); covHash != "" {
		covPath := strings.TrimSpace(toString(cfg["coverage_harness_path"]))
		if covPath != "" {
			if err := hunt.PublishHarnessFile(ctx, a.db, covHash, covPath, sourceRel+"#afl"); err != nil {
				return fmt.Errorf("hunt publish coverage harness: %w", err)
			}
			cfg["coverage_harness_fetch_path"] = hunt.HarnessFetchURL(covHash)
		}
	}
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
	srcRel := strings.TrimSpace(toString(cfg["hunt_source_rel"]))
	if err := poolsync.UploadHuntHarness(ctx, coord, token, hash, data, srcRel); err != nil {
		// Catalog hash is metadata-stable; clang rebuilds can differ byte-for-byte across hosts.
		// "already bound" from the coordinator means that hash is already published and fetchable.
		if poolsync.IsHarnessAlreadyBound(err) {
			// continue to coverage harness upload
		} else {
			return err
		}
	}
	if covHash := strings.TrimSpace(toString(cfg["coverage_harness_hash"])); covHash != "" {
		covData, cerr := hunt.GetHarnessArtifact(ctx, a.db, covHash)
		if cerr != nil {
			// Publish from on-disk path if not yet in local artifact DB.
			if covPath := strings.TrimSpace(toString(cfg["coverage_harness_path"])); covPath != "" {
				if perr := hunt.PublishHarnessFile(ctx, a.db, covHash, covPath, srcRel+"#afl"); perr == nil {
					covData, cerr = hunt.GetHarnessArtifact(ctx, a.db, covHash)
				}
			}
		}
		if cerr == nil && len(covData) > 0 {
			if uerr := poolsync.UploadHuntHarness(ctx, coord, token, covHash, covData, srcRel+"#afl"); uerr != nil && !poolsync.IsHarnessAlreadyBound(uerr) {
				return fmt.Errorf("hunt coverage harness sync: %w", uerr)
			}
		}
	}
	return nil
}
