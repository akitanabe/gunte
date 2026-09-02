package compile

import (
	"bytes"
	"sort"

	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/lexer"
	"github.com/akitanabe/gunte/internal/source"
)

type splice struct {
	start       int
	end         int
	replacement []byte
	outputStart int
	outputEnd   int
}

func projectSource(project config.ProjectConfig, targetID string, unit SourceUnit) SourceProjection {
	excluded := excludedRanges(targetID, unit.IR.OnlyBlocks)
	splices := projectionSplices(unit, excluded, projectionValues(project, targetID))
	projected, applied := applySplices(unit.Document.Buffer, unit.Document.BodyRange, splices)

	result := SourceProjection{Path: unit.Path, Bytes: projected}
	result.Contracts = make([]ProjectedDeclaration, 0, len(unit.IR.ContractSpans))
	for _, span := range unit.IR.ContractSpans {
		result.Contracts = append(result.Contracts, ProjectedDeclaration{
			ID:             span.Token,
			Source:         sourcePosition(unit.Path, span.Open.Position),
			Emitted:        true,
			ProjectedRange: source.Range{Start: projectedOffset(span.ContentRange.Start, unit.Document.BodyRange.Start, applied), End: projectedOffset(span.ContentRange.End, unit.Document.BodyRange.Start, applied)},
		})
	}
	result.Anchors = make([]ProjectedDeclaration, 0, len(unit.IR.Anchors))
	for _, anchor := range unit.IR.Anchors {
		emitted := !containedByAny(anchor.Range, excluded)
		declaration := ProjectedDeclaration{ID: anchor.Token, Source: sourcePosition(unit.Path, anchor.Position), Emitted: emitted}
		if emitted {
			offset := projectedOffset(anchor.Range.Start, unit.Document.BodyRange.Start, applied)
			declaration.ProjectedRange = source.Range{Start: offset, End: offset}
		}
		result.Anchors = append(result.Anchors, declaration)
	}
	return result
}

func excludedRanges(targetID string, blocks []lexer.Block) []source.Range {
	result := make([]source.Range, 0, len(blocks))
	for _, block := range blocks {
		if block.Token == targetID || block.Close == nil {
			continue
		}
		result = append(result, source.Range{Start: block.Open.Range.Start, End: block.Close.Range.End})
	}
	return result
}

func projectionSplices(unit SourceUnit, excluded []source.Range, values map[string]string) []splice {
	splices := make([]splice, 0, len(unit.IR.Markers)+len(unit.IR.TermUses)+len(excluded))
	for _, deletion := range excluded {
		splices = append(splices, splice{start: deletion.Start, end: deletion.End})
	}
	for _, marker := range unit.IR.Markers {
		if !containedByAny(marker.Range, excluded) {
			splices = append(splices, splice{start: marker.Range.Start, end: marker.Range.End})
		}
	}
	for _, term := range unit.IR.TermUses {
		if !containedByAny(term.Range, excluded) {
			splices = append(splices, splice{start: term.Range.Start, end: term.Range.End, replacement: []byte(values[term.Name])})
		}
	}
	sort.SliceStable(splices, func(i, j int) bool {
		if splices[i].start == splices[j].start {
			return splices[i].end > splices[j].end
		}
		return splices[i].start < splices[j].start
	})
	return splices
}

func termValues(terms []config.Term, targetID string) map[string]string {
	values := make(map[string]string, len(terms))
	for _, term := range terms {
		for _, value := range term.Values {
			if value.TargetID == targetID {
				values[term.Name] = value.Value
				break
			}
		}
	}
	return values
}

func projectionValues(project config.ProjectConfig, targetID string) map[string]string {
	values := termValues(project.Terms, targetID)
	for _, bodyValue := range project.BodyValues {
		if bodyValue.From == "project:version" {
			values[bodyValue.Name] = project.Project.Version
		}
	}
	return values
}

func applySplices(buffer []byte, body source.Range, splices []splice) ([]byte, []splice) {
	var output bytes.Buffer
	output.Grow(body.End - body.Start)
	cursor := body.Start
	applied := make([]splice, 0, len(splices))
	for _, edit := range splices {
		if edit.start < cursor || edit.start < body.Start || edit.end > body.End {
			continue
		}
		output.Write(buffer[cursor:edit.start])
		edit.outputStart = output.Len()
		output.Write(edit.replacement)
		edit.outputEnd = output.Len()
		applied = append(applied, edit)
		cursor = edit.end
	}
	output.Write(buffer[cursor:body.End])
	return output.Bytes(), applied
}

func projectedOffset(offset, bodyStart int, splices []splice) int {
	delta := -bodyStart
	for _, edit := range splices {
		switch {
		case offset < edit.start:
			return offset + delta
		case offset == edit.start:
			return edit.outputStart
		case offset < edit.end:
			return edit.outputStart
		case offset == edit.end:
			return edit.outputEnd
		default:
			delta += len(edit.replacement) - (edit.end - edit.start)
		}
	}
	return offset + delta
}

func containedByAny(candidate source.Range, containers []source.Range) bool {
	for _, container := range containers {
		if candidate.Start >= container.Start && candidate.End <= container.End {
			return true
		}
	}
	return false
}
