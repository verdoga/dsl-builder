package model

import (
	"encoding/json"
	"testing"
)

func TestFormatVersionsHaveContractValuesAndJSONStrings(t *testing.T) {
	tests := []struct {
		name     string
		version  FormatVersion
		want     string
		wantJSON string
	}{
		{name: "FormatVersion1", version: FormatVersion1, want: "1.0", wantJSON: `"1.0"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.version) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.version, test.want)
			}
			encoded, err := json.Marshal(test.version)
			if err != nil {
				t.Fatalf("JSON-маршалинг %s: %v", test.name, err)
			}
			if string(encoded) != test.wantJSON {
				t.Errorf("JSON %s = %s, требуется %s", test.name, encoded, test.wantJSON)
			}
		})
	}
}

func TestLineTypesHaveContractValuesAndJSONStrings(t *testing.T) {
	tests := []struct {
		name     string
		lineType LineType
		want     string
		wantJSON string
	}{
		{name: "LineTypeTag", lineType: LineTypeTag, want: "tag", wantJSON: `"tag"`},
		{name: "LineTypeHeading", lineType: LineTypeHeading, want: "heading", wantJSON: `"heading"`},
		{name: "LineTypeContent", lineType: LineTypeContent, want: "content", wantJSON: `"content"`},
		{name: "LineTypeBlockStart", lineType: LineTypeBlockStart, want: "block-start", wantJSON: `"block-start"`},
		{name: "LineTypeBlockEnd", lineType: LineTypeBlockEnd, want: "block-end", wantJSON: `"block-end"`},
		{name: "LineTypeSeparator", lineType: LineTypeSeparator, want: "separator", wantJSON: `"separator"`},
		{name: "LineTypeBlank", lineType: LineTypeBlank, want: "blank", wantJSON: `"blank"`},
		{name: "LineTypeInvalid", lineType: LineTypeInvalid, want: "invalid", wantJSON: `"invalid"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.lineType) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.lineType, test.want)
			}
			encoded, err := json.Marshal(test.lineType)
			if err != nil {
				t.Fatalf("JSON-маршалинг %s: %v", test.name, err)
			}
			if string(encoded) != test.wantJSON {
				t.Errorf("JSON %s = %s, требуется %s", test.name, encoded, test.wantJSON)
			}
		})
	}
}

func TestLineEndingsHaveContractValuesAndJSONEscaping(t *testing.T) {
	tests := []struct {
		name     string
		ending   LineEnding
		want     string
		wantJSON string
	}{
		{name: "LineEndingLF", ending: LineEndingLF, want: "\n", wantJSON: `"\n"`},
		{name: "LineEndingCRLF", ending: LineEndingCRLF, want: "\r\n", wantJSON: `"\r\n"`},
		{name: "LineEndingNone", ending: LineEndingNone, want: "", wantJSON: `""`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.ending) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.ending, test.want)
			}
			encoded, err := json.Marshal(test.ending)
			if err != nil {
				t.Fatalf("JSON-маршалинг %s: %v", test.name, err)
			}
			if string(encoded) != test.wantJSON {
				t.Errorf("JSON %s = %s, требуется %s", test.name, encoded, test.wantJSON)
			}
		})
	}
}

func TestElementTypesHaveContractValuesAndJSONStrings(t *testing.T) {
	tests := []struct {
		name        string
		elementType ElementType
		want        string
		wantJSON    string
	}{
		{name: "ElementTypeTag", elementType: ElementTypeTag, want: "tag", wantJSON: `"tag"`},
		{name: "ElementTypeHeadingLevel", elementType: ElementTypeHeadingLevel, want: "heading-level", wantJSON: `"heading-level"`},
		{name: "ElementTypeTitle", elementType: ElementTypeTitle, want: "title", wantJSON: `"title"`},
		{name: "ElementTypeContent", elementType: ElementTypeContent, want: "content", wantJSON: `"content"`},
		{name: "ElementTypeIdentifier", elementType: ElementTypeIdentifier, want: "identifier", wantJSON: `"identifier"`},
		{name: "ElementTypeName", elementType: ElementTypeName, want: "name", wantJSON: `"name"`},
		{name: "ElementTypeVersion", elementType: ElementTypeVersion, want: "version", wantJSON: `"version"`},
		{name: "ElementTypeNumber", elementType: ElementTypeNumber, want: "number", wantJSON: `"number"`},
		{name: "ElementTypeMediaType", elementType: ElementTypeMediaType, want: "media-type", wantJSON: `"media-type"`},
		{name: "ElementTypeSource", elementType: ElementTypeSource, want: "source", wantJSON: `"source"`},
		{name: "ElementTypeResourcePath", elementType: ElementTypeResourcePath, want: "resource-path", wantJSON: `"resource-path"`},
		{name: "ElementTypePlaceholder", elementType: ElementTypePlaceholder, want: "placeholder", wantJSON: `"placeholder"`},
		{name: "ElementTypeUnparsed", elementType: ElementTypeUnparsed, want: "unparsed", wantJSON: `"unparsed"`},
		{name: "ElementTypeBlockOpen", elementType: ElementTypeBlockOpen, want: "block-open", wantJSON: `"block-open"`},
		{name: "ElementTypeBlockClose", elementType: ElementTypeBlockClose, want: "block-close", wantJSON: `"block-close"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.elementType) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.elementType, test.want)
			}
			encoded, err := json.Marshal(test.elementType)
			if err != nil {
				t.Fatalf("JSON-маршалинг %s: %v", test.name, err)
			}
			if string(encoded) != test.wantJSON {
				t.Errorf("JSON %s = %s, требуется %s", test.name, encoded, test.wantJSON)
			}
		})
	}
}
