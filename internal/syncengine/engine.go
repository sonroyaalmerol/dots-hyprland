package syncengine

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/syncengine/hyprparse"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/syncengine/kvparse"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/syncengine/sectionparse"
)

type SyncStep struct {
	UpstreamPath string
	DeployPath   string
	RelPath      string
	Strategy     SyncStrategy
}

type SyncDecision int

const (
	DecisionNoop SyncDecision = iota
	DecisionUpdate
	DecisionKeep
	DecisionConflict
	DecisionNew
)

type SyncResult struct {
	RelPath  string
	Decision SyncDecision
	Strategy SyncStrategy
	Err      error
	Conflict *ConflictInfo
}

type Config struct {
	ManifestPath string
	Variables    TemplateVars
	Categorizer  *Categorizer
}

type SyncEngine struct {
	cfg      Config
	manifest *Manifest
}

func New(cfg Config) *SyncEngine {
	return &SyncEngine{cfg: cfg}
}

func (e *SyncEngine) Run(ctx context.Context, steps []SyncStep) []SyncResult {
	m, err := LoadManifest(e.cfg.ManifestPath)
	if err != nil {
		m = &Manifest{Version: "1", Entries: make(map[string]*FileEntry)}
	}
	e.manifest = m

	results := make([]SyncResult, 0, len(steps))
	for _, step := range steps {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		result := e.syncFile(ctx, step)
		results = append(results, result)

		if err := SaveManifest(e.manifest, e.cfg.ManifestPath); err != nil {
			fmt.Fprintf(os.Stderr, "  [sync] warning: failed to save manifest: %v\n", err)
		}
	}

	return results
}

func (e *SyncEngine) syncFile(_ context.Context, step SyncStep) SyncResult {
	upstream, err := os.ReadFile(step.UpstreamPath)
	if err != nil {
		return SyncResult{RelPath: step.RelPath, Err: fmt.Errorf("read upstream %s: %w", step.UpstreamPath, err)}
	}

	var current []byte
	currentExists := true
	currentData, err := os.ReadFile(step.DeployPath)
	if err != nil {
		if os.IsNotExist(err) {
			currentExists = false
		} else {
			return SyncResult{RelPath: step.RelPath, Err: fmt.Errorf("read current %s: %w", step.DeployPath, err)}
		}
	}
	current = currentData

	entry := e.manifest.GetEntry(step.RelPath)

	strategy := step.Strategy
	if strategy == "" {
		if e.cfg.Categorizer != nil {
			strategy = e.cfg.Categorizer.Categorize(step.RelPath)
		} else {
			strategy = StrategyOverwrite
		}
	}

	if strategy == StrategyTemplate && HasTemplateVariables(upstream) {
		upstream = RenderTemplate(upstream, e.cfg.Variables)
	}

	upstreamSHA := sha256Of(upstream)
	var currentSHA string
	if currentExists {
		currentSHA = sha256Of(current)
	}

	origSHA := ""
	if entry != nil {
		origSHA = entry.OriginalSHA256
	}

	decision := decide(origSHA, currentSHA, upstreamSHA)

	var result []byte
	var conflict *ConflictInfo

	switch strategy {
	case StrategyOverwrite:
		result, conflict = handleOverwrite(decision, current, upstream)

	case StrategySkipIfExists:
		result, conflict = handleSkipIfExists(decision, current, upstream)

	case StrategyMergeKV:
		result, conflict = e.handleMergeKV(step, decision, origSHA, currentSHA, upstreamSHA, current, upstream)

	case StrategyMergeHyprland:
		result, conflict = e.handleMergeHyprland(step, decision, origSHA, currentSHA, upstreamSHA, current, upstream)

	case StrategyMergeSection:
		result, conflict = e.handleMergeSection(step, decision, origSHA, currentSHA, upstreamSHA, current, upstream)

	case StrategyTemplate:
		result, conflict = handleOverwrite(decision, current, upstream)

	default:
		result, conflict = handleOverwrite(decision, current, upstream)
	}

	if decision != DecisionNoop && decision != DecisionKeep {
		if err := os.MkdirAll(filepathDir(step.DeployPath), 0o755); err != nil {
			return SyncResult{RelPath: step.RelPath, Decision: decision, Strategy: strategy, Err: fmt.Errorf("mkdir: %w", err)}
		}
		if err := atomicWrite(step.DeployPath, result, 0o644); err != nil {
			return SyncResult{RelPath: step.RelPath, Decision: decision, Strategy: strategy, Err: fmt.Errorf("write: %w", err)}
		}
	}

	newEntry := &FileEntry{
		RelPath:        step.RelPath,
		Strategy:       string(strategy),
		OriginalSHA256: upstreamSHA,
		CurrentSHA256:  upstreamSHA,
		UpstreamSHA256: upstreamSHA,
		DeployPath:     step.DeployPath,
		UpstreamPath:   step.UpstreamPath,
		LastSynced:     time.Now().UTC().Format(time.RFC3339),
		Conflict:       conflict != nil,
	}
	e.manifest.SetEntry(newEntry)

	return SyncResult{
		RelPath:  step.RelPath,
		Decision: decision,
		Strategy: strategy,
		Conflict: conflict,
	}
}

func decide(origSHA, currentSHA, upstreamSHA string) SyncDecision {
	if currentSHA == "" {
		return DecisionNew
	}

	currentUnchanged := currentSHA == origSHA
	upstreamUnchanged := upstreamSHA == origSHA

	if currentUnchanged && upstreamUnchanged {
		return DecisionNoop
	}
	if currentUnchanged && !upstreamUnchanged {
		return DecisionUpdate
	}
	if !currentUnchanged && upstreamUnchanged {
		return DecisionKeep
	}
	return DecisionConflict
}

func handleOverwrite(decision SyncDecision, current, upstream []byte) ([]byte, *ConflictInfo) {
	switch decision {
	case DecisionNoop, DecisionKeep:
		return current, nil
	default:
		return upstream, nil
	}
}

func handleSkipIfExists(decision SyncDecision, current, upstream []byte) ([]byte, *ConflictInfo) {
	if decision == DecisionNew {
		return upstream, nil
	}
	return current, nil
}

func (e *SyncEngine) handleMergeKV(step SyncStep, decision SyncDecision, origSHA, currentSHA, upstreamSHA string, current, upstream []byte) ([]byte, *ConflictInfo) {
	switch decision {
	case DecisionNoop, DecisionKeep:
		return current, nil
	case DecisionUpdate:
		return upstream, nil
	case DecisionNew:
		return upstream, nil
	}

	origData := []byte{}
	if origSHA != "" {
		origData = readOrigFromManifest(step, origSHA)
	}

	merged, conflicts, err := kvparse.MergeKV(origData, current, upstream)
	if err != nil {
		conflict := &ConflictInfo{
			RelPath:     step.RelPath,
			Strategy:    string(step.Strategy),
			Reason:      fmt.Sprintf("kv merge error: %v", err),
			OrigSHA:     origSHA,
			CurrentSHA:  currentSHA,
			UpstreamSHA: upstreamSHA,
		}
		e.handleConflict(step, current, upstream, conflict)
		return current, conflict
	}

	if len(conflicts) > 0 {
		conflict := &ConflictInfo{
			RelPath:     step.RelPath,
			Strategy:    string(step.Strategy),
			Reason:      fmt.Sprintf("kv conflict: keys %v", conflicts),
			OrigSHA:     origSHA,
			CurrentSHA:  currentSHA,
			UpstreamSHA: upstreamSHA,
		}
		e.handleConflict(step, current, upstream, conflict)
		return current, conflict
	}

	return merged, nil
}

func (e *SyncEngine) handleMergeHyprland(step SyncStep, decision SyncDecision, origSHA, currentSHA, upstreamSHA string, current, upstream []byte) ([]byte, *ConflictInfo) {
	switch decision {
	case DecisionNoop, DecisionKeep:
		return current, nil
	case DecisionUpdate:
		return upstream, nil
	case DecisionNew:
		return upstream, nil
	}

	origData := []byte{}
	if origSHA != "" {
		origData = readOrigFromManifest(step, origSHA)
	}

	merged, conflicts, err := hyprparse.ThreeWayMerge(origData, current, upstream)
	if err != nil {
		conflict := &ConflictInfo{
			RelPath:     step.RelPath,
			Strategy:    string(step.Strategy),
			Reason:      fmt.Sprintf("hyprland merge error: %v", err),
			OrigSHA:     origSHA,
			CurrentSHA:  currentSHA,
			UpstreamSHA: upstreamSHA,
		}
		e.handleConflict(step, current, upstream, conflict)
		return current, conflict
	}

	if len(conflicts) > 0 {
		conflict := &ConflictInfo{
			RelPath:     step.RelPath,
			Strategy:    string(step.Strategy),
			Reason:      fmt.Sprintf("hyprland conflict: keys %v", conflicts),
			OrigSHA:     origSHA,
			CurrentSHA:  currentSHA,
			UpstreamSHA: upstreamSHA,
		}
		e.handleConflict(step, current, upstream, conflict)
		return current, conflict
	}

	return merged, nil
}

func (e *SyncEngine) handleMergeSection(step SyncStep, decision SyncDecision, origSHA, currentSHA, upstreamSHA string, current, upstream []byte) ([]byte, *ConflictInfo) {
	switch decision {
	case DecisionNoop, DecisionKeep:
		return current, nil
	case DecisionUpdate:
		if sectionparse.HasMarker(current) {
			block := sectionparse.ExtractBlock(upstream)
			return sectionparse.ReplaceBlock(current, block), nil
		}
		return upstream, nil
	case DecisionNew:
		return sectionparse.WrapInMarkers(upstream), nil
	}

	origData := []byte{}
	if origSHA != "" {
		origData = readOrigFromManifest(step, origSHA)
	}

	merged, conflicts, err := sectionparse.MergeSection(origData, current, upstream)
	if err != nil {
		conflict := &ConflictInfo{
			RelPath:     step.RelPath,
			Strategy:    string(step.Strategy),
			Reason:      fmt.Sprintf("section merge error: %v", err),
			OrigSHA:     origSHA,
			CurrentSHA:  currentSHA,
			UpstreamSHA: upstreamSHA,
		}
		e.handleConflict(step, current, upstream, conflict)
		return current, conflict
	}

	if len(conflicts) > 0 {
		conflict := &ConflictInfo{
			RelPath:     step.RelPath,
			Strategy:    string(step.Strategy),
			Reason:      fmt.Sprintf("section conflict: keys %v", conflicts),
			OrigSHA:     origSHA,
			CurrentSHA:  currentSHA,
			UpstreamSHA: upstreamSHA,
		}
		e.handleConflict(step, current, upstream, conflict)
		return current, conflict
	}

	return merged, nil
}

func (e *SyncEngine) handleConflict(step SyncStep, current, upstream []byte, conflict *ConflictInfo) {
	_ = WriteConflictBackups(step.DeployPath, current, upstream)
	conflictsPath := filepathDir(step.DeployPath) + "/../snry-shell/conflicts.jsonl"
	_ = LogConflict(conflictsPath, *conflict)
}

func readOrigFromManifest(step SyncStep, origSHA string) []byte {
	data, err := os.ReadFile(step.DeployPath + ".orig")
	if err != nil {
		return []byte{}
	}
	if sha256Of(data) == origSHA {
		return data
	}
	return []byte{}
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
