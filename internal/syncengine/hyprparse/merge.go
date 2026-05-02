package hyprparse

import (
	"sort"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/syncengine/kvparse"
)

func ThreeWayMerge(orig, current, upstream []byte) ([]byte, []string, error) {
	origDoc := Parse(orig)
	currentDoc := Parse(current)
	upstreamDoc := Parse(upstream)

	result := &Document{}
	var conflicts []string

	upstreamSections := AllSectionNames(upstreamDoc)
	currentSections := AllSectionNames(currentDoc)
	origSections := AllSectionNames(origDoc)

	origSectionSet := make(map[string]bool)
	for _, name := range origSections {
		origSectionSet[name] = true
	}
	currentSectionSet := make(map[string]bool)
	for _, name := range currentSections {
		currentSectionSet[name] = true
	}
	upstreamSectionSet := make(map[string]bool)
	for _, name := range upstreamSections {
		upstreamSectionSet[name] = true
	}

	processedSections := make(map[string]bool)

	// 1. Walk sections present in upstream
	for _, sName := range upstreamSections {
		processedSections[sName] = true
		upSection := FindSection(upstreamDoc, sName)
		origSection := FindSection(origDoc, sName)
		currentSection := FindSection(currentDoc, sName)

		if origSection == nil {
			result.Children = append(result.Children, upSection)
			continue
		}

		origKV := SectionKeyValueMap(origSection)
		currentKV := map[string]string{}
		if currentSection != nil {
			currentKV = SectionKeyValueMap(currentSection)
		}
		upKV := SectionKeyValueMap(upSection)

		mergedKV, sectionConflicts := kvparse.ThreeWayMergeKV(origKV, currentKV, upKV)
		conflicts = append(conflicts, sectionConflicts...)

		newSection := &Section{Name: sName}
		if currentSection != nil {
			for _, child := range currentSection.Children {
				if kv, ok := child.(*KeyValue); ok {
					if val, exists := mergedKV[kv.Key]; exists {
						newSection.Children = append(newSection.Children, &KeyValue{
							Key:     kv.Key,
							Value:   val,
							Comment: kv.Comment,
						})
						delete(mergedKV, kv.Key)
					}
				} else {
					newSection.Children = append(newSection.Children, child)
				}
			}
			for key, val := range mergedKV {
				newSection.Children = append(newSection.Children, &KeyValue{Key: key, Value: val})
			}
		} else {
			result.Children = append(result.Children, upSection)
			continue
		}
		result.Children = append(result.Children, newSection)
	}

	// 2. Sections in current not in upstream
	for _, sName := range currentSections {
		if processedSections[sName] {
			continue
		}
		currentSection := FindSection(currentDoc, sName)
		if currentSection == nil {
			continue
		}

		if origSectionSet[sName] && !upstreamSectionSet[sName] {
			continue
		}

		result.Children = append(result.Children, currentSection)
		processedSections[sName] = true
	}

	// 3. Top-level key-value entries
	origTopKV := TopLevelKeyValue(origDoc)
	currentTopKV := TopLevelKeyValue(currentDoc)
	upTopKV := TopLevelKeyValue(upstreamDoc)

	mergedTopKV, topConflicts := kvparse.ThreeWayMergeKV(origTopKV, currentTopKV, upTopKV)
	conflicts = append(conflicts, topConflicts...)

	resultKVNodes := buildKVNodes(currentDoc, mergedTopKV)

	// 4. Top-level bind/exec lines
	origBinds := TopLevelBinds(origDoc)
	currentBinds := TopLevelBinds(currentDoc)
	upBinds := TopLevelBinds(upstreamDoc)

	mergedBinds := threeWayMergeLinesBind(origBinds, currentBinds, upBinds)

	origExecs := TopLevelExecs(origDoc)
	currentExecs := TopLevelExecs(currentDoc)
	upExecs := TopLevelExecs(upstreamDoc)

	mergedExecs := threeWayMergeLinesExec(origExecs, currentExecs, upExecs)

	// 5. Source directives
	currentSources := Sources(currentDoc)
	upSources := Sources(upstreamDoc)
	mergedSources := mergeSources(currentSources, upSources)

	// 6. Rebuild top-level: preserve ordering from current doc
	finalChildren := buildTopLevelNodes(resultKVNodes, mergedBinds, mergedExecs, mergedSources, currentDoc)

	// Prepend non-section top-level nodes, then sections
	_ = finalChildren
	var topNodes []Node
	topNodes = append(topNodes, finalChildren...)
	topNodes = append(topNodes, result.Children...)
	result.Children = topNodes

	if len(conflicts) == 0 {
		conflicts = nil
	}

	return Serialize(result), conflicts, nil
}

func buildKVNodes(currentDoc *Document, merged map[string]string) []Node {
	var nodes []Node
	used := make(map[string]bool)

	for _, child := range currentDoc.Children {
		if kv, ok := child.(*KeyValue); ok {
			if val, exists := merged[kv.Key]; exists {
				nodes = append(nodes, &KeyValue{Key: kv.Key, Value: val, Comment: kv.Comment})
				used[kv.Key] = true
			}
		} else if _, isSection := child.(*Section); !isSection {
			nodes = append(nodes, child)
		}
	}

	var newKeys []string
	for k := range merged {
		if !used[k] {
			newKeys = append(newKeys, k)
		}
	}
	sort.Strings(newKeys)
	for _, k := range newKeys {
		nodes = append(nodes, &KeyValue{Key: k, Value: merged[k]})
	}

	return nodes
}

func threeWayMergeLinesBind(orig, current, upstream []*BindEntry) []*BindEntry {
	origSet := make(map[string]bool)
	for _, b := range orig {
		origSet[b.RawLine] = true
	}
	currentSet := make(map[string]bool)
	for _, b := range current {
		currentSet[b.RawLine] = true
	}
	upstreamSet := make(map[string]bool)
	for _, b := range upstream {
		upstreamSet[b.RawLine] = true
	}

	var result []*BindEntry
	resultSet := make(map[string]bool)

	for _, b := range upstream {
		result = append(result, b)
		resultSet[b.RawLine] = true
	}

	for _, b := range current {
		if !origSet[b.RawLine] && !upstreamSet[b.RawLine] && !resultSet[b.RawLine] {
			result = append(result, b)
			resultSet[b.RawLine] = true
		}
	}

	return result
}

func threeWayMergeLinesExec(orig, current, upstream []*ExecEntry) []*ExecEntry {
	origSet := make(map[string]bool)
	for _, e := range orig {
		origSet[e.RawLine] = true
	}
	currentSet := make(map[string]bool)
	for _, e := range current {
		currentSet[e.RawLine] = true
	}
	upstreamSet := make(map[string]bool)
	for _, e := range upstream {
		upstreamSet[e.RawLine] = true
	}

	var result []*ExecEntry
	resultSet := make(map[string]bool)

	for _, e := range upstream {
		result = append(result, e)
		resultSet[e.RawLine] = true
	}

	for _, e := range current {
		if !origSet[e.RawLine] && !upstreamSet[e.RawLine] && !resultSet[e.RawLine] {
			result = append(result, e)
			resultSet[e.RawLine] = true
		}
	}

	return result
}

func mergeSources(current, upstream []*SourceDirective) []*SourceDirective {
	currentPaths := make(map[string]bool)
	for _, s := range current {
		currentPaths[s.Path] = true
	}

	var result []*SourceDirective
	resultSet := make(map[string]bool)

	for _, s := range current {
		result = append(result, s)
		resultSet[s.RawLine] = true
	}

	for _, s := range upstream {
		if !currentPaths[s.Path] && !resultSet[s.RawLine] {
			result = append(result, s)
			resultSet[s.RawLine] = true
		}
	}

	return result
}

func buildTopLevelNodes(kvNodes []Node, binds []*BindEntry, execs []*ExecEntry, sources []*SourceDirective, currentDoc *Document) []Node {
	var result []Node
	_ = currentDoc

	result = append(result, kvNodes...)
	for _, b := range binds {
		result = append(result, b)
	}
	for _, e := range execs {
		result = append(result, e)
	}
	for _, s := range sources {
		result = append(result, s)
	}

	return result
}
