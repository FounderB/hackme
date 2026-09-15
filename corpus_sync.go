package main

import (
	"context"
	"log"

	"hackme/internal/fuzzengine"
	"hackme/internal/poolfuzz"
	"hackme/internal/poolsync"
)

func (a *app) syncCorpusNamespaceToCoordinator(ctx context.Context, cfg map[string]any) {
	if a == nil || a.db == nil || cfg == nil || !fuzzengine.CorpusPersistEnabled(cfg) {
		return
	}
	ns := fuzzengine.CorpusPersistNamespace(cfg)
	if ns == "" {
		return
	}
	coord := poolsync.ResolveCoordinatorURL()
	token := poolsync.AdminToken()
	if coord == "" || token == "" {
		return
	}
	svc := &poolfuzz.Service{DB: a.db}
	seeds, err := svc.ListNamespaceCorpus(ctx, ns, fuzzengine.CorpusPersistMax(cfg))
	if err != nil || len(seeds) == 0 {
		return
	}
	_ = poolsync.UploadCorpusNamespace(ctx, coord, token, ns, seeds)
}

func (a *app) publishDigPoolArtifacts(ctx context.Context, cfg map[string]any) {
	a.syncCorpusNamespaceToCoordinator(ctx, cfg)
	if poolfuzz.IsHuntCampaign(cfg) {
		if err := a.syncHuntHarnessToCoordinator(ctx, cfg); err != nil {
			log.Printf("publishDigPoolArtifacts: hunt harness sync: %v", err)
		}
	}
}
