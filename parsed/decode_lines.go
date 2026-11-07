package parsed

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// decodeLine читает обязательные поля строки и ненулевой массив элементов.
// Физический номер из входа уточняет контекст последующих ошибок.
func decodeLine(raw json.RawMessage, field string, context diagnostic.Context) (model.Line, *diagnostic.Error) {
	fields, err := decodeObject(raw, []string{
		"line", "type", "nestingLevel", "parentLine", "raw", "eol", "hasErrors", "elements",
	}, field, context)
	if err != nil {
		return model.Line{}, err
	}
	prefix := field
	if prefix != "" {
		prefix += "."
	}
	var line model.Line
	if err := decodeScalar(fields["line"], &line.Number, false, prefix+"line", context); err != nil {
		return model.Line{}, err
	}
	if line.Number > 0 {
		context.Line, context.Column = line.Number, 0
	}
	if err := decodeScalar(fields["type"], &line.LineType, false, prefix+"type", context); err != nil {
		return model.Line{}, err
	}
	if err := decodeScalar(fields["nestingLevel"], &line.NestingLevel, false, prefix+"nestingLevel", context); err != nil {
		return model.Line{}, err
	}
	if err := decodeScalar(fields["parentLine"], &line.ParentLine, true, prefix+"parentLine", context); err != nil {
		return model.Line{}, err
	}
	if err := decodeScalar(fields["raw"], &line.Raw, false, prefix+"raw", context); err != nil {
		return model.Line{}, err
	}
	if err := decodeScalar(fields["eol"], &line.LineEnding, false, prefix+"eol", context); err != nil {
		return model.Line{}, err
	}
	if err := decodeScalar(fields["hasErrors"], &line.HasErrors, false, prefix+"hasErrors", context); err != nil {
		return model.Line{}, err
	}
	items, err := decodeArray(fields["elements"], false, prefix+"elements", context)
	if err != nil {
		return model.Line{}, err
	}
	line.Elements = make([]model.Element, len(items))
	for i, item := range items {
		line.Elements[i], err = decodeElement(item, fmt.Sprintf("%selements[%d]", prefix, i), context)
		if err != nil {
			return model.Line{}, err
		}
	}
	return line, nil
}

// decodeElement читает обязательные поля элемента, сохраняя string|null
// семантического value без повторного снятия экранирования DSL.
func decodeElement(raw json.RawMessage, field string, context diagnostic.Context) (model.Element, *diagnostic.Error) {
	fields, err := decodeObject(raw, []string{"type", "raw", "value", "start", "end", "errorIds"}, field, context)
	if err != nil {
		return model.Element{}, err
	}
	prefix := field
	if prefix != "" {
		prefix += "."
	}
	var element model.Element
	if err := decodeScalar(fields["type"], &element.ElementType, false, prefix+"type", context); err != nil {
		return model.Element{}, err
	}
	if err := decodeScalar(fields["raw"], &element.Raw, false, prefix+"raw", context); err != nil {
		return model.Element{}, err
	}
	if err := decodeScalar(fields["value"], &element.Value, true, prefix+"value", context); err != nil {
		return model.Element{}, err
	}
	if err := decodeScalar(fields["start"], &element.Start, false, prefix+"start", context); err != nil {
		return model.Element{}, err
	}
	if context.Line > 0 && element.Start > 0 {
		context.Column = element.Start
	}
	if err := decodeScalar(fields["end"], &element.End, false, prefix+"end", context); err != nil {
		return model.Element{}, err
	}
	element.ErrorIDs, err = decodeStrings(fields["errorIds"], false, prefix+"errorIds", context)
	if err != nil {
		return model.Element{}, err
	}
	return element, nil
}

// decodeDiagnostic читает обязательные поля диагностики, nullable location
// и ненулевой массив ненулевых relatedLocations в исходном порядке.
func decodeDiagnostic(raw json.RawMessage, field string, context diagnostic.Context) (model.Diagnostic, *diagnostic.Error) {
	fields, err := decodeObject(raw, []string{
		"id", "source", "code", "severity", "message", "scope", "fatal", "location", "relatedLocations",
	}, field, context)
	if err != nil {
		return model.Diagnostic{}, err
	}
	prefix := field
	if prefix != "" {
		prefix += "."
	}
	var item model.Diagnostic
	if err := decodeScalar(fields["id"], &item.ID, false, prefix+"id", context); err != nil {
		return model.Diagnostic{}, err
	}
	if err := decodeScalar(fields["source"], &item.Source, false, prefix+"source", context); err != nil {
		return model.Diagnostic{}, err
	}
	if err := decodeScalar(fields["code"], &item.DiagnosticCode, false, prefix+"code", context); err != nil {
		return model.Diagnostic{}, err
	}
	if err := decodeScalar(fields["severity"], &item.SeverityLevel, false, prefix+"severity", context); err != nil {
		return model.Diagnostic{}, err
	}
	if err := decodeScalar(fields["message"], &item.Message, false, prefix+"message", context); err != nil {
		return model.Diagnostic{}, err
	}
	if err := decodeScalar(fields["scope"], &item.DiagnosticScope, false, prefix+"scope", context); err != nil {
		return model.Diagnostic{}, err
	}
	if err := decodeScalar(fields["fatal"], &item.Fatal, false, prefix+"fatal", context); err != nil {
		return model.Diagnostic{}, err
	}
	if !bytes.Equal(bytes.TrimSpace(fields["location"]), []byte("null")) {
		location, err := decodeLocation(fields["location"], prefix+"location", context)
		if err != nil {
			return model.Diagnostic{}, err
		}
		item.Location = &location
		if location.Start.Line > 0 {
			context.Line, context.Column = location.Start.Line, 0
			if location.Start.Column > 0 {
				context.Column = location.Start.Column
			}
		}
	}
	locations, err := decodeArray(fields["relatedLocations"], false, prefix+"relatedLocations", context)
	if err != nil {
		return model.Diagnostic{}, err
	}
	item.RelatedLocations = make([]model.Location, len(locations))
	for i, location := range locations {
		item.RelatedLocations[i], err = decodeLocation(location, fmt.Sprintf("%srelatedLocations[%d]", prefix, i), context)
		if err != nil {
			return model.Diagnostic{}, err
		}
	}
	return item, nil
}

// decodeLocation читает обязательные ненулевые start/end через decodePosition.
// Допустимость null основной location определяет decodeDiagnostic.
func decodeLocation(raw json.RawMessage, field string, context diagnostic.Context) (model.Location, *diagnostic.Error) {
	fields, err := decodeObject(raw, []string{"start", "end"}, field, context)
	if err != nil {
		return model.Location{}, err
	}
	prefix := field
	if prefix != "" {
		prefix += "."
	}
	var location model.Location
	location.Start, err = decodePosition(fields["start"], prefix+"start", context)
	if err != nil {
		return model.Location{}, err
	}
	if location.Start.Line > 0 {
		context.Line, context.Column = location.Start.Line, 0
		if location.Start.Column > 0 {
			context.Column = location.Start.Column
		}
	}
	location.End, err = decodePosition(fields["end"], prefix+"end", context)
	if err != nil {
		return model.Location{}, err
	}
	return location, nil
}

// decodePosition читает обязательные целые line/column ненулевого объекта.
// Положительность и отношения координат проверяются после декодирования.
func decodePosition(raw json.RawMessage, field string, context diagnostic.Context) (model.Position, *diagnostic.Error) {
	fields, err := decodeObject(raw, []string{"line", "column"}, field, context)
	if err != nil {
		return model.Position{}, err
	}
	prefix := field
	if prefix != "" {
		prefix += "."
	}
	var position model.Position
	if err := decodeScalar(fields["line"], &position.Line, false, prefix+"line", context); err != nil {
		return model.Position{}, err
	}
	if position.Line > 0 {
		context.Line, context.Column = position.Line, 0
	}
	if err := decodeScalar(fields["column"], &position.Column, false, prefix+"column", context); err != nil {
		return model.Position{}, err
	}
	return position, nil
}
