package sectionparse

import (
	"github.com/sonroyaalmerol/snry-shell-qs/internal/syncengine/kvparse"
)

func MergeSection(orig, current, upstream []byte) ([]byte, []string, error) {
	if !HasMarker(orig) && !HasMarker(current) {
		return WrapInMarkers(upstream), nil, nil
	}

	origBlock := ExtractBlock(orig)
	currentBlock := ExtractBlock(current)
	upstreamBlock := ExtractBlock(upstream)

	if kvparse.IsKVContent(origBlock) && kvparse.IsKVContent(currentBlock) && kvparse.IsKVContent(upstreamBlock) {
		merged, conflicts, err := kvparse.MergeKV(origBlock, currentBlock, upstreamBlock)
		if err != nil {
			return ReplaceBlock(current, upstreamBlock), nil, nil
		}
		result := ReplaceBlock(current, merged)
		return result, conflicts, nil
	}

	return ReplaceBlock(current, upstreamBlock), nil, nil
}
