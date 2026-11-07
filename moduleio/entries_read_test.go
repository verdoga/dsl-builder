package moduleio

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"errors"
	"hash/crc32"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func EntriesRawReader(t *testing.T, method uint16, payload []byte, checksum uint32, size uint64) (*zip.Reader, []byte) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	stream, err := writer.CreateRaw(&zip.FileHeader{
		Name: "l001.json", Method: method, CRC32: checksum,
		CompressedSize64: uint64(len(payload)), UncompressedSize64: size,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), buffer.Bytes()...)
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return reader, raw
}

func TestEntriesReadExactBytesThroughEOF(t *testing.T) {
	context := diagnostic.Context{Path: "Модули/Unit 1.zip"}
	for _, payload := range [][]byte{nil, []byte("{ \"unknown\": \"Точный текст\" }\r\n"), append([]byte("{}"), bytes.Repeat([]byte(" \t\r\n"), 32768)...), {0xff, 0, 0xef, 0xbb, 0xbf}} {
		reader, raw := EntriesRawReader(t, zip.Store, payload, crc32.ChecksumIEEE(payload), uint64(len(payload)))
		before := append([]byte(nil), raw...)
		got, problems := readEntry(reader.File[0], context)
		if len(problems) != 0 || !bytes.Equal(got, payload) {
			t.Fatalf("exact uncompressed bytes not preserved: length=%d errors=%v", len(got), problems)
		}
		if !bytes.Equal(before, raw) {
			t.Fatal("archive bytes modified")
		}
	}
	payload := append([]byte("{}"), bytes.Repeat([]byte(" \r\n"), 32768)...)
	var compressed bytes.Buffer
	compressor, err := flate.NewWriter(&compressed, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compressor.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	reader, _ := EntriesRawReader(t, zip.Deflate, compressed.Bytes(), crc32.ChecksumIEEE(payload), uint64(len(payload)))
	got, problems := readEntry(reader.File[0], context)
	if len(problems) != 0 || !bytes.Equal(got, payload) {
		t.Fatalf("deflated entry not fully read: %v", problems)
	}
}

func TestEntriesReadRejectsCorruptionAndUnsupportedCompression(t *testing.T) {
	const privateBody = "private-body-Qk9EWS1TRUNSRVQ="
	payload := append([]byte(`{"data":"`+privateBody+`"}`), bytes.Repeat([]byte(" "), 65536)...)
	for _, tc := range []struct {
		name     string
		method   uint16
		checksum uint32
		size     uint64
		cause    error
	}{
		{"CRC after valid JSON prefix", zip.Store, crc32.ChecksumIEEE(payload) ^ 1, uint64(len(payload)), zip.ErrChecksum},
		{"size too large", zip.Store, crc32.ChecksumIEEE(payload), uint64(len(payload) + 1), io.ErrUnexpectedEOF},
		{"size too small", zip.Store, crc32.ChecksumIEEE(payload), uint64(len(payload) - 1), zip.ErrFormat},
		{"unsupported method", 62000, crc32.ChecksumIEEE(payload), uint64(len(payload)), zip.ErrAlgorithm},
		{"invalid deflate", zip.Deflate, crc32.ChecksumIEEE(payload), uint64(len(payload)), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, _ := EntriesRawReader(t, tc.method, payload, tc.checksum, tc.size)
			got, problems := readEntry(reader.File[0], diagnostic.Context{Path: "corrupt.zip"})
			if got != nil {
				t.Fatal("read failure returned partial bytes")
			}
			problem := EntriesProblem(t, problems, CodeRead, "l001.json")
			if problem.Context.Path != "corrupt.zip" || problem.Cause == nil {
				t.Fatalf("read cause/context lost: %#v", problem)
			}
			if tc.cause != nil && !errors.Is(problem, tc.cause) {
				t.Fatalf("missing archive/zip cause %v: %v", tc.cause, problem.Cause)
			}
			if strings.Contains(problem.Reason, privateBody) {
				t.Fatal("entry body copied into reason")
			}
			for _, detail := range problem.Details {
				if strings.Contains(detail.Value, privateBody) {
					t.Fatal("entry body copied into details")
				}
			}
		})
	}
}

type EntriesStream struct {
	reader     *bytes.Reader
	readError  error
	closeError error
	closed     int
	finalRead  bool
}

func (stream *EntriesStream) Read(dst []byte) (int, error) {
	count, err := stream.reader.Read(dst)
	if stream.reader.Len() == 0 {
		stream.finalRead = true
		if stream.readError != nil {
			return count, stream.readError
		}
	}
	return count, err
}

func (stream *EntriesStream) Close() error {
	stream.closed++
	return stream.closeError
}

func TestEntriesReadClosesAndPreservesBothErrors(t *testing.T) {
	readFailure := errors.New("ошибка последнего чтения")
	closeFailure := errors.New("ошибка закрытия потока")
	payload := []byte("{}")
	for _, tc := range []struct {
		name                  string
		readError, closeError error
		badCRC                bool
		codes                 []string
		causes                []error
	}{
		{"success", nil, nil, false, nil, nil},
		{"read", readFailure, nil, false, []string{CodeRead}, []error{readFailure}},
		{"close", nil, closeFailure, false, []string{CodeClose}, []error{closeFailure}},
		{"read and close", readFailure, closeFailure, false, []string{CodeRead, CodeClose}, []error{readFailure, closeFailure}},
		{"CRC and close", nil, closeFailure, true, []string{CodeRead, CodeClose}, []error{zip.ErrChecksum, closeFailure}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checksum := crc32.ChecksumIEEE(payload)
			if tc.badCRC {
				checksum ^= 1
			}
			reader, _ := EntriesRawReader(t, 62001, payload, checksum, uint64(len(payload)))
			stream := &EntriesStream{reader: bytes.NewReader(payload), readError: tc.readError, closeError: tc.closeError}
			reader.RegisterDecompressor(62001, func(io.Reader) io.ReadCloser { return stream })
			got, problems := readEntry(reader.File[0], diagnostic.Context{Path: "close.zip"})
			if stream.closed != 1 || !stream.finalRead {
				t.Fatalf("stream was not fully read and closed: %#v", stream)
			}
			if len(problems) != len(tc.codes) {
				t.Fatalf("read/close errors: %v", problems)
			}
			if len(tc.codes) == 0 {
				if !bytes.Equal(got, payload) {
					t.Fatal("successful read lost bytes")
				}
			} else if got != nil {
				t.Fatal("failure returned partial bytes")
			}
			for i, problem := range problems {
				if problem.Code != tc.codes[i] || !errors.Is(problem, tc.causes[i]) || problem.Context.Path != "close.zip" || problem.Context.FileName != "l001.json" {
					t.Fatalf("error %d lost cause/order/context: %#v", i, problem)
				}
			}
		})
	}
}
