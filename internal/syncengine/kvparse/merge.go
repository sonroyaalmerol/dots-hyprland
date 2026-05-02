package kvparse

import "sort"

func ThreeWayMergeKV(orig, current, upstream map[string]string) (map[string]string, []string) {
	result := make(map[string]string)
	allKeys := make(map[string]bool)
	var conflicts []string

	for k := range orig {
		allKeys[k] = true
	}
	for k := range current {
		allKeys[k] = true
	}
	for k := range upstream {
		allKeys[k] = true
	}

	sortedKeys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, key := range sortedKeys {
		inOrig, inCurrent, inUpstream := false, false, false
		valOrig, valCurrent, valUpstream := "", "", ""

		if v, ok := orig[key]; ok {
			inOrig = true
			valOrig = v
		}
		if v, ok := current[key]; ok {
			inCurrent = true
			valCurrent = v
		}
		if v, ok := upstream[key]; ok {
			inUpstream = true
			valUpstream = v
		}

		switch {
		case !inOrig && !inCurrent && inUpstream:
			result[key] = valUpstream

		case !inOrig && inCurrent && !inUpstream:
			result[key] = valCurrent

		case !inOrig && inCurrent && inUpstream:
			if valCurrent == valUpstream {
				result[key] = valCurrent
			} else {
				result[key] = valCurrent
				conflicts = append(conflicts, key)
			}

		case inOrig && !inCurrent && !inUpstream:
			// deleted by both

		case inOrig && !inCurrent && inUpstream:
			// user deleted, upstream changed -> delete (user intent wins)

		case inOrig && inCurrent && !inUpstream:
			if valCurrent != valOrig {
				result[key] = valCurrent
			}
			// upstream deleted and user didn't modify -> delete

		case inOrig && inCurrent && inUpstream:
			if valCurrent == valOrig {
				result[key] = valUpstream
			} else if valUpstream == valOrig {
				result[key] = valCurrent
			} else {
				result[key] = valCurrent
				conflicts = append(conflicts, key)
			}
		}
	}

	return result, conflicts
}

func MergeKV(orig, current, upstream []byte) ([]byte, []string, error) {
	origDoc := Parse(orig)
	currentDoc := Parse(current)
	upstreamDoc := Parse(upstream)

	origMap := origDoc.AllKeyValueMap()
	currentMap := currentDoc.AllKeyValueMap()
	upstreamMap := upstreamDoc.AllKeyValueMap()

	merged, conflicts := ThreeWayMergeKV(origMap, currentMap, upstreamMap)

	resultDoc := &KVDocument{
		Comments: currentDoc.Comments,
	}

	sectionKeys := make(map[string]bool)
	for _, s := range currentDoc.Sections {
		sectionKeys[s.Name] = true
	}
	for _, s := range upstreamDoc.Sections {
		sectionKeys[s.Name] = true
	}

	sortedSections := make([]string, 0, len(sectionKeys))
	for s := range sectionKeys {
		sortedSections = append(sortedSections, s)
	}
	sort.Strings(sortedSections)

	for _, sectionName := range sortedSections {
		var section KVSection
		section.Name = sectionName

		keysInSection := make(map[string]bool)
		for _, s := range currentDoc.Sections {
			if s.Name == sectionName {
				for _, e := range s.Entries {
					if e.Key != "" {
						keysInSection[e.Key] = true
					}
				}
			}
		}
		for _, s := range upstreamDoc.Sections {
			if s.Name == sectionName {
				for _, e := range s.Entries {
					if e.Key != "" {
						keysInSection[e.Key] = true
					}
				}
			}
		}

		sortedEntryKeys := make([]string, 0, len(keysInSection))
		for k := range keysInSection {
			sortedEntryKeys = append(sortedEntryKeys, k)
		}
		sort.Strings(sortedEntryKeys)

		for _, key := range sortedEntryKeys {
			if val, ok := merged[key]; ok {
				section.Entries = append(section.Entries, KVEntry{
					Key:   key,
					Value: val,
				})
			}
		}

		if sectionName == "" || len(section.Entries) > 0 {
			resultDoc.Sections = append(resultDoc.Sections, section)
		}
	}

	return Serialize(resultDoc), conflicts, nil
}
