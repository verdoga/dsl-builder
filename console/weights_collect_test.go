package console

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"strings"
	"testing"
)

func TestWeightsCollectPreservesRawMemberIdentityAndItemsOrder(t *testing.T) {
	huge := strings.Repeat("9", 130)
	for _, test := range []struct {
		name    string
		section *string
		want    map[string]string
	}{
		{"section_value_not_pointer", weightsTestString("Бета"), map[string]string{"l004.json": huge, "l007.json": "0000123"}},
		{"nil_group", nil, map[string]string{"l009.json": huge, "l001.json": "0000123"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := weightsTestDTO()
			before := weightsTestJSON(t, snapshot.Catalog)
			d := weightsTestDialog(nil, strings.NewReader("1\n"+huge+"\n1\n0000123\n"), io.Discard)
			got, accepted, err := d.collectLessonWeights(snapshot, test.section)
			if err != nil || !accepted || !maps.Equal(got, test.want) {
				t.Fatalf("collect = %#v, %v, %v; want %#v", got, accepted, err, test.want)
			}
			if !bytes.Equal(before, weightsTestJSON(t, snapshot.Catalog)) {
				t.Fatal("collection mutated snapshot")
			}
		})
	}
}

func TestWeightsCollectDoesNotValidateRawWeights(t *testing.T) {
	for _, raw := range []string{"0", "-1", "+2", "1.5", "1e4", " 003 ", "１２", "\t7"} {
		t.Run(raw, func(t *testing.T) {
			d := weightsTestDialog(nil, strings.NewReader("1\n"+raw+"\n1\n0002\n"), io.Discard)
			got, accepted, err := d.collectLessonWeights(weightsTestDTO(), weightsTestString("Бета"))
			if err != nil || !accepted || got["l004.json"] != raw || got["l007.json"] != "0002" || len(got) != 2 {
				t.Fatalf("raw value was validated or changed: %#v, %v, %v", got, accepted, err)
			}
		})
	}
}

func TestWeightsCollectCancelDiscardsEveryEarlierValue(t *testing.T) {
	for _, script := range []string{"2\n", "1\n0001\n2\n"} {
		t.Run(script, func(t *testing.T) {
			d := weightsTestDialog(nil, strings.NewReader(script), io.Discard)
			got, accepted, err := d.collectLessonWeights(weightsTestDTO(), weightsTestString("Бета"))
			if err != nil || accepted || got != nil {
				t.Fatalf("cancel leaked local set: %#v, %v, %v", got, accepted, err)
			}
		})
	}
}

// weightsTestFaultReader возвращает заданный сбой после полного префикса ввода.
type weightsTestFaultReader struct {
	input *strings.Reader
	cause error
	calls int
}

func (reader *weightsTestFaultReader) Read(dst []byte) (int, error) {
	reader.calls++
	if reader.input.Len() != 0 {
		return reader.input.Read(dst)
	}
	return 0, reader.cause
}

// weightsTestFaultWriter моделирует стандартную границу ошибки вывода.
type weightsTestFaultWriter struct {
	cause error
	short bool
	calls int
}

func (writer *weightsTestFaultWriter) Write(raw []byte) (int, error) {
	writer.calls++
	if writer.short {
		return 0, nil
	}
	return 0, writer.cause
}

func TestWeightsCollectEOFAndReadFailureNeverReturnPartialMap(t *testing.T) {
	failure := errors.New("weights input failed")
	for _, test := range []struct {
		name  string
		input io.Reader
		want  error
	}{
		{"before_first", strings.NewReader(""), io.EOF},
		{"partial_first", strings.NewReader("1\n123"), io.EOF},
		{"after_previous", strings.NewReader("1\n123\n"), io.EOF},
		{"partial_second", strings.NewReader("1\n123\n1\n456"), io.EOF},
		{"stream_error", &weightsTestFaultReader{input: strings.NewReader("1\n123\n"), cause: failure}, failure},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := weightsTestDialog(nil, test.input, io.Discard)
			got, accepted, err := d.collectLessonWeights(weightsTestDTO(), weightsTestString("Бета"))
			if got != nil || accepted || !errors.Is(err, test.want) {
				t.Fatalf("partial result: %#v, %v, %v", got, accepted, err)
			}
			if test.want != io.EOF && !isStreamFailure(err) {
				t.Fatal("read failure lost stream classification")
			}
		})
	}
}

func TestWeightsCollectOutputFailureStopsBeforeReading(t *testing.T) {
	failure := errors.New("weights output failed")
	for _, short := range []bool{false, true} {
		reader := &weightsTestFaultReader{input: strings.NewReader("1\n99\n1\n88\n"), cause: io.EOF}
		writer := &weightsTestFaultWriter{cause: failure, short: short}
		d := weightsTestDialog(nil, reader, writer)
		got, accepted, err := d.collectLessonWeights(weightsTestDTO(), weightsTestString("Бета"))
		want := failure
		if short {
			want = io.ErrShortWrite
		}
		if got != nil || accepted || !errors.Is(err, want) || !isStreamFailure(err) {
			t.Fatalf("output failure: %#v, %v, %v", got, accepted, err)
		}
		if reader.calls != 0 || writer.calls != 1 {
			t.Fatalf("continued after failure: reader=%d writer=%d", reader.calls, writer.calls)
		}
	}
}
