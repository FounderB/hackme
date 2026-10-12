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

// sanitizePinRoot cleans pinPath and rebuilds it via the allowlist barrier so
// subsequent joins/reads are not treated as path-injection sinks (CodeQL).
func sanitizePinRoot(pinPath string) (string, error) {
	pinPath = strings.TrimSpace(pinPath)
	if pinPath == "" {
		return "", nil
	}
	abs, err := filepath.Abs(filepath.Clean(pinPath))
	if err != nil {
		return "", err
	}
	return allowlistedAbs(abs)
}

// safeInventoryFileName rejects path separators / traversal in dir entry names.
func safeInventoryFileName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	base := filepath.Base(name)
	if base != name {
		return "", false
	}
	if strings.ContainsAny(base, `/\`) {
		return "", false
	}
	return base, true
}

// ImportInventoryPackSeeds copies public corpus/*.bin from a pinned inventory repo
// into the L2 libFuzzer seed cache (excludes crash-/bug-heap style crashers).
// Returns newly written seed count.
func ImportInventoryPackSeeds(repoRoot, targetID, pinPath string) (int, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return 0, nil
	}
	safePin, err := sanitizePinRoot(pinPath)
	if err != nil || safePin == "" {
		return 0, err
	}
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	ents, err := SafeReadDirUnder(safePin, "corpus")
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
		name, ok := safeInventoryFileName(ent.Name())
		if !ok {
			continue
		}
		low := strings.ToLower(name)
		if !strings.HasSuffix(low, ".bin") && !strings.HasSuffix(low, ".seed") {
			continue
		}
		if strings.Contains(low, "bug_heap") || strings.Contains(low, "crash") ||
			strings.HasPrefix(low, ".") {
			continue
		}
		b, rerr := SafeReadFileUnder(safePin, filepath.Join("corpus", name))
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
	safePin, err := sanitizePinRoot(pinPath)
	if err != nil || safePin == "" {
		return nil, "", err
	}
	candidates := []string{}
	sourceRel = filepath.Clean(strings.TrimSpace(sourceRel))
	if sourceRel != "" && sourceRel != "." && !filepath.IsAbs(sourceRel) &&
		!strings.HasPrefix(sourceRel, ".."+string(os.PathSeparator)) && sourceRel != ".." {
		dir := filepath.Dir(sourceRel)
		base := strings.TrimSuffix(filepath.Base(sourceRel), filepath.Ext(sourceRel))
		if baseName, ok := safeInventoryFileName(base + ".dict"); ok {
			if dir == "." {
				candidates = append(candidates, baseName)
			} else {
				candidates = append(candidates, filepath.Join(dir, baseName))
			}
		}
		if dir == "." {
			candidates = append(candidates, "fuzz.dict")
		} else {
			candidates = append(candidates, filepath.Join(dir, "fuzz.dict"))
		}
	}
	for _, rel := range []string{
		"fuzz/heavywire.dict",
		"fuzz/fuzz.dict",
		"heavywire.dict",
		"fuzz.dict",
	} {
		candidates = append(candidates, rel)
	}
	// Also any single *.dict under fuzz/
	if ents, err := SafeReadDirUnder(safePin, "fuzz"); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			name, ok := safeInventoryFileName(e.Name())
			if !ok {
				continue
			}
			if strings.HasSuffix(strings.ToLower(name), ".dict") {
				candidates = append(candidates, filepath.Join("fuzz", name))
			}
		}
	}
	seen := map[string]struct{}{}
	for _, rel := range candidates {
		rel = filepath.Clean(strings.TrimSpace(rel))
		if rel == "" || rel == "." || filepath.IsAbs(rel) ||
			rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		st, err := SafeStatUnder(safePin, rel)
		if err != nil || st.IsDir() || st.Size() <= 0 || st.Size() > maxInventoryDictBytes {
			continue
		}
		b, err := SafeReadFileUnder(safePin, rel)
		if err != nil || len(b) == 0 {
			continue
		}
		return b, rel, nil
	}
	return nil, "", nil
}

// ApplyInventoryPackAssets imports corpus seeds + libFuzzer dict from the pin into cfg.
func ApplyInventoryPackAssets(cfg map[string]any, repoRoot, targetID, pinPath, sourceRel string) error {
	if cfg == nil {
		return nil
	}
	safePin, err := sanitizePinRoot(pinPath)
	if err != nil {
		return err
	}
	if safePin == "" {
		return nil
	}
	// Prefer public pack corpus files as the L2 head (avoid burying TLV under stale LF dumps).
	corpusDir, joinErr := SafeJoinUnder(safePin, "corpus")
	if joinErr == nil {
		if seeds, err := LoadLibFuzzerSeedFiles(corpusDir, maxInventoryPackSeeds); err == nil && len(seeds) > 0 {
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
	}
	if _, err := ImportInventoryPackSeeds(repoRoot, targetID, safePin); err != nil {
		return fmt.Errorf("hunt: inventory pack seeds: %w", err)
	}
	if _, err := MergeLibFuzzerSeedCorpus(cfg, repoRoot, targetID); err != nil {
		return fmt.Errorf("hunt: inventory seed merge: %w", err)
	}
	if _, ok := cfg["libfuzzer_dict"]; !ok {
		if dict, rel, err := LoadInventoryLibFuzzerDict(safePin, sourceRel); err != nil {
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
