package poolfuzz

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"hackme/internal/fuzzengine"
	"hackme/internal/sandbox"
)

// Dig CLEAN hygiene must not pay from miner-supplied check_result alone.
// When WASM is present (or escrow Dig), the coordinator invokes check once on the
// locked lease input and only then may accept CLEAN / enqueue PayRun.
//
// Env:
//
//	HACKME_POOL_HYGIENE_PROVE  default ON (0/false/off = legacy trust-miner CLEAN; ops rollback only)
//	HACKME_POOL_HYGIENE_SEGMENT_PROVE
//	  "" / auto — full segment prove on Dig hygiene for dig_package=deep or escrow Dig with exec_per>1
//	  1/true/on — full segment prove for all Dig hygiene multi-exec (Hunt untouched)
//	  0/false/off — single-anchor prove only (C1)

func hygieneProveEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_POOL_HYGIENE_PROVE")))
	if v == "" {
		return true
	}
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// hygieneSegmentProveApplies reports whether Dig hygiene_skip must prove the full
// segment (exec_per_unit) instead of the locked anchor alone. Hunt never applies.
func hygieneSegmentProveApplies(cfg map[string]any, execPer int) bool {
	if IsHuntCampaign(cfg) || execPer <= 1 {
		return false
	}
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_POOL_HYGIENE_SEGMENT_PROVE")))
	switch v {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	default: // auto
		if digPackageKey(cfg) == "deep" {
			return true
		}
		return escrowEnabled(cfg)
	}
}

// proveHygieneAnchorCheck runs one coordinator WASM invoke on the locked input.
// Returns coordinator check_result / pass. recordFinding on the anchor means the
// miner should have claimed a finding (hygiene path rejects — no silent pay).
func (s *Service) proveHygieneAnchorCheck(ctx context.Context, cfg map[string]any, sem fuzzengine.CheckSemantics, expectedU uint64, expectedB []byte) (checkResult int32, trap string, pass bool, recordFinding bool, err error) {
	wasmHex := wasmHexFromConfig(cfg)
	if wasmHex == "" {
		if escrowEnabled(cfg) && !IsHuntCampaign(cfg) {
			return 0, "", false, false, fmt.Errorf("poolfuzz: escrow Dig hygiene requires wasm_check_hex")
		}
		// Non-escrow Dig without WASM cannot be proven — refuse CLEAN accept.
		return 0, "", false, false, fmt.Errorf("poolfuzz: Dig hygiene requires wasm_check_hex prove")
	}
	wasm, err := hex.DecodeString(wasmHex)
	if err != nil || len(wasm) == 0 {
		return 0, "", false, false, fmt.Errorf("poolfuzz: invalid campaign wasm")
	}
	if vErr := sandbox.ValidateCheckWasm(ctx, wasm); vErr != nil {
		return 0, "", false, false, fmt.Errorf("poolfuzz: invalid campaign wasm: %w", vErr)
	}
	timeoutMS := sandbox.Policy().CheckTimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	inB := fuzzengine.ClampInputBytes(expectedB, cfg)
	var execErr error
	if len(inB) > 0 {
		out, e := sandbox.InvokeCheckOutcomeInput(runCtx, wasm, inB)
		execErr = e
		if e != nil {
			trap = e.Error()
		} else if out.OK {
			checkResult = 1
		}
	} else {
		out, e := sandbox.InvokeCheckOutcome(runCtx, wasm, expectedU)
		execErr = e
		if e != nil {
			trap = e.Error()
		} else if out.OK {
			checkResult = 1
		}
	}
	pass, recordFinding = fuzzengine.EvalCheck(sem, checkResult, execErr)
	return checkResult, trap, pass, recordFinding, nil
}
