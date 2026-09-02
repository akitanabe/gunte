package compile

import (
	"fmt"
	"regexp"

	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/lexer"
	"github.com/akitanabe/gunte/internal/source"
)

var (
	generalIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	targetIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
)

// ValidateAndProject validates declarations across the complete configured
// source set, then calculates every target projection. Inputs are not modified.
func ValidateAndProject(project config.ProjectConfig, units []SourceUnit) (Result, []source.Diagnostic) {
	diagnostics := validateSourceUnits(project, units)
	if len(diagnostics) != 0 {
		return Result{}, diagnostics
	}

	result := Result{Targets: make([]TargetProjection, 0, len(project.Targets))}
	for _, target := range project.Targets {
		projection := TargetProjection{TargetID: target.ID, Sources: make([]SourceProjection, 0, len(units))}
		for _, unit := range units {
			projection.Sources = append(projection.Sources, projectSource(project, target.ID, unit))
		}
		result.Targets = append(result.Targets, projection)
	}
	return result, nil
}

func validateSourceUnits(project config.ProjectConfig, units []SourceUnit) []source.Diagnostic {
	var diagnostics []source.Diagnostic
	for index := 0; index < len(project.Sources.Files) || index < len(units); index++ {
		switch {
		case index >= len(units):
			path := project.Sources.Files[index]
			diagnostics = append(diagnostics, source.Diagnostic{Path: path, Line: 1, Column: 1, Message: "missing source unit " + path})
		case index >= len(project.Sources.Files):
			path := units[index].Path
			diagnostics = append(diagnostics, source.Diagnostic{Path: path, Line: 1, Column: 1, Message: "unexpected source unit " + path})
		case units[index].Path != project.Sources.Files[index]:
			diagnostics = append(diagnostics, source.Diagnostic{
				Path: units[index].Path, Line: 1, Column: 1,
				Message: fmt.Sprintf("source unit path %s does not match configured path %s", units[index].Path, project.Sources.Files[index]),
			})
		}
	}

	targets := make(map[string]bool, len(project.Targets))
	for _, target := range project.Targets {
		targets[target.ID] = true
	}
	terms := make(map[string]bool, len(project.Terms))
	for _, term := range project.Terms {
		terms[term.Name] = true
	}
	for _, bodyValue := range project.BodyValues {
		terms[bodyValue.Name] = true
	}
	declarations := map[string]SourcePosition{}
	for _, unit := range units {
		for _, marker := range unit.IR.Markers {
			switch marker.Kind {
			case lexer.ContractOpen, lexer.AnchorMarker:
				position := sourcePosition(unit.Path, marker.Position)
				if !generalIDPattern.MatchString(marker.Token) {
					diagnostics = append(diagnostics, diagnosticAt(position, "invalid span or anchor ID "+marker.Token))
				}
				if _, exists := declarations[marker.Token]; exists {
					diagnostics = append(diagnostics, diagnosticAt(position, "duplicate span or anchor ID "+marker.Token))
				} else {
					declarations[marker.Token] = position
				}
			case lexer.OnlyOpen:
				position := sourcePosition(unit.Path, marker.Position)
				if !targetIDPattern.MatchString(marker.Token) {
					diagnostics = append(diagnostics, diagnosticAt(position, "invalid only target "+marker.Token))
				} else if !targets[marker.Token] {
					diagnostics = append(diagnostics, diagnosticAt(position, "unknown only target "+marker.Token))
				}
			}
		}
		for _, term := range unit.IR.TermUses {
			if !terms[term.Name] {
				position := sourcePosition(unit.Path, term.Position)
				diagnostics = append(diagnostics, diagnosticAt(position, "undefined term "+term.Name))
			}
		}
	}
	return diagnostics
}

func sourcePosition(path string, position lexer.Position) SourcePosition {
	return SourcePosition{Path: path, Offset: position.Offset, Line: position.Line, Column: position.Column}
}

func diagnosticAt(position SourcePosition, message string) source.Diagnostic {
	return source.Diagnostic{
		Path: position.Path, Offset: position.Offset, Line: position.Line, Column: position.Column, Message: message,
	}
}
