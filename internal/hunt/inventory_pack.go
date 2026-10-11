package hunt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxInventoryPackSeeds = 64
	maxInventoryDictBytes = 256 * 1024
)

// ImportInventoryPackSeeds copies public corpus/*.bin from a pinned inventory repo
// into the L2 libFuzzer seed cache (excludes crash-/bug-heap style crashers).
// Returns newly written seed count.
func ImportInventoryPackSeeds(repoRoot, targetID, pinPath string) (int, error) {
	pinPath = strings.TrimSpace(pinPath)
	targetID = strings.TrimSpace(targetID)
	if pinPath == "" || targetID == "" {
		return 0, nil
	}
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	corpusDir := filepath.Join(pinPath, "corpus")
	ents, err := os.ReadDir(corpusDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	seeds := make([][]byte, 0, maxInventoryPackSeeds)
	for _, ent := range ents {
		if len(seeds) >= maxInventoryPackSeeds {
			break
		}
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		low := strings.ToLower(name)
		if !strings.HasSuffix(low, ".bin") && !strings.HasSuffix(low, ".seed") {
			continue
		}
		if strings.Contains(low, "bug_heap") || strings.Contains(low, "crash") ||
			strings.HasPrefix(low, ".") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(corpusDir, name))
		if rerr != nil || len(b) == 0 || len(b) > libFuzzerSeedMaxBytes {
			continue
		}
		seeds = append(seeds, b)
	}
	if len(seeds) == 0 {
		return 0, nil
	}
	return ExportLibFuzzerSeeds(repoRoot, targetID, seeds)
}

// LoadInventoryLibFuzzerDict finds a public *.dict next to the harness or under fuzz/.
func LoadInventoryLibFuzzerDict(pinPath, sourceRel string) ([]byte, string, error) {
	pinPath = strings.TrimSpace(pinPath)
	if pinPath == "" {
		return nil, "", nil
	}
	candidates := []string{}
	if sourceRel != "" {
		dir := filepath.Dir(sourceRel)
		base := strings.TrimSuffix(filepath.Base(sourceRel), filepath.Ext(sourceRel))
		candidates = append(candidates,
			filepath.Join(pinPath, dir, base+".dict"),
			filepath.Join(pinPath, dir, "fuzz.dict"),
		)
	}
	for _, rel := range []string{
		"fuzz/heavywire.dict",
		"fuzz/fuzz.dict",
		"heavywire.dict",
		"fuzz.dict",
	} {
		candidates = append(candidates, filepath.Join(pinPath, rel))
	}
	// Also any single *.dict under fuzz/
	if ents, err := os.ReadDir(filepath.Join(pinPath, "fuzz")); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(strings.ToLower(e.Name()), ".dict") {
				candidates = append(candidates, filepath.Join(pinPath, "fuzz", e.Name()))
			}
		}
	}
	seen := map[string]struct{}{}
	for _, p := range candidates {
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		st, err := os.Stat(p)
		if err != nil || st.IsDir() || st.Size() <= 0 || st.Size() > maxInventoryDictBytes {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			continue
		}
		rel, _ := filepath.Rel(pinPath, p)
		return b, rel, nil
	}
	return nil, "", nil
}

// ApplyInventoryPackAssets imports corpus seeds + libFuzzer dict from the pin into cfg.
func ApplyInventoryPackAssets(cfg map[string]any, repoRoot, targetID, pinPath, sourceRel string) error {
	if cfg == nil {
		return nil
	}
	// Prefer public pack corpus files as the L2 head (avoid burying TLV under stale LF dumps).
	if seeds, err := LoadLibFuzzerSeedFiles(filepath.Join(strings.TrimSpace(pinPath), "corpus"), maxInventoryPackSeeds); err == nil && len(seeds) > 0 {
		filtered := make([][]byte, 0, len(seeds))
		for _, b := range seeds {
			if len(b) == 0 {
				continue
			}
			filtered = append(filtered, b)
		}
		if len(filtered) > 0 {
			_ = mergeSeedByteCorpus(cfg, filtered)
		}
	}
	if _, err := ImportInventoryPackSeeds(repoRoot, targetID, pinPath); err != nil {
		return fmt.Errorf("hunt: inventory pack seeds: %w", err)
	}
	if _, err := MergeLibFuzzerSeedCorpus(cfg, repoRoot, targetID); err != nil {
		return fmt.Errorf("hunt: inventory seed merge: %w", err)
	}
	if _, ok := cfg["libfuzzer_dict"]; !ok {
		if dict, rel, err := LoadInventoryLibFuzzerDict(pinPath, sourceRel); err != nil {
			return err
		} else if len(dict) > 0 {
			cfg["libfuzzer_dict"] = string(dict)
			cfg["libfuzzer_dict_rel"] = rel
		}
	}
	return nil
}

// LibFuzzerDictFromConfig returns AFL/libFuzzer dictionary text from campaign cfg.
func LibFuzzerDictFromConfig(cfg map[string]any) []byte {
	if cfg == nil {
		return nil
	}
	switch v := cfg["libfuzzer_dict"].(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			return []byte(s)
		}
	case []byte:
		if len(v) > 0 {
			return v
		}
	}
	return nil
}
