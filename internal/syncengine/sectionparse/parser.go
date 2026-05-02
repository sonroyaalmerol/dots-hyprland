package sectionparse

import (
	"bytes"
)

var (
	markerStart = []byte("# >>> snry-shell >>>")
	markerEnd   = []byte("# <<< snry-shell <<<")
)

func HasMarker(data []byte) bool {
	return bytes.Contains(data, markerStart) && bytes.Contains(data, markerEnd)
}

func ExtractBlock(data []byte) []byte {
	startIdx := bytes.Index(data, markerStart)
	if startIdx < 0 {
		return nil
	}

	startIdx = startIdx + len(markerStart)
	if startIdx < len(data) && data[startIdx] == '\n' {
		startIdx++
	}

	endIdx := bytes.Index(data[startIdx:], markerEnd)
	if endIdx < 0 {
		return data[startIdx:]
	}

	block := data[startIdx : startIdx+endIdx]
	if len(block) > 0 && block[len(block)-1] == '\n' {
		block = block[:len(block)-1]
	}
	return block
}

func ReplaceBlock(data []byte, block []byte) []byte {
	startIdx := bytes.Index(data, markerStart)
	if startIdx < 0 {
		return WrapInMarkers(block)
	}

	endIdx := bytes.Index(data[startIdx:], markerEnd)
	if endIdx < 0 {
		return WrapInMarkers(block)
	}

	endIdx = startIdx + endIdx + len(markerEnd)

	var buf bytes.Buffer
	buf.Write(data[:startIdx])
	buf.Write(markerStart)
	buf.WriteByte('\n')
	buf.Write(block)
	buf.WriteByte('\n')
	buf.Write(markerEnd)
	if endIdx < len(data) {
		buf.Write(data[endIdx:])
	}

	return buf.Bytes()
}

func WrapInMarkers(data []byte) []byte {
	var buf bytes.Buffer
	buf.Write(markerStart)
	buf.WriteByte('\n')
	buf.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.Write(markerEnd)
	buf.WriteByte('\n')
	return buf.Bytes()
}
