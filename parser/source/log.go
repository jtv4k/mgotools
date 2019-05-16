package source

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"sync"
	"unicode"

	"mgotools/internal"
	"mgotools/parser/record"
)

var ErrorParsingDate = errors.New("unrecognized date format")
var ErrorMissingContext = errors.New("missing context")

type Log struct {
	io.Closer
	*bufio.Reader
	*bufio.Scanner

	next  record.Base
	error error

	closed bool
	eof    bool
	line   uint
	mutex  sync.RWMutex
}

// Enforce the interface at compile time.
var _ Factory = (*Log)(nil)

func NewLog(base io.ReadCloser) (*Log, error) {
	reader := bufio.NewReader(base)

	if scanner, err := makeScanner(reader); err != nil {
		return nil, err
	} else {
		return &Log{
			Reader:  reader,
			Closer:  base,
			Scanner: scanner,

			// These are all defaults, but it doesn't hurts to be explicit.
			closed: false,
			eof:    false,
			line:   0,
			mutex:  sync.RWMutex{},
		}, nil
	}
}

func makeScanner(reader *bufio.Reader) (*bufio.Scanner, error) {
	var scanner = bufio.NewScanner(reader)

	// Check for gzip magic headers.
	if peek, err := reader.Peek(2); err == nil {
		if peek[0] == 0x1f && peek[1] == 0x8b {
			if gzipReader, err := gzip.NewReader(reader); err == nil {
				scanner = bufio.NewScanner(gzipReader)
			} else {
				return nil, err
			}
		}
	}
	return scanner, nil
}

// Generate an Entry from a line of text. This method assumes the entry is *not* JSON.
func (Log) NewBase(line string, num uint) (record.Base, error) {
	var (
		base = record.Base{RuneReader: internal.NewRuneReader(line), LineNumber: num, Severity: record.SeverityNone}
		pos  int
	)

	// Check for a day in the first portion of the string, which represents version <= 2.4
	if day := base.PreviewWord(1); internal.IsDay(day) {
		base.RawDate = parseCDateString(&base)
		base.CString = true
	} else if internal.IsIso8601String(base.PreviewWord(1)) {
		base.RawDate, _ = base.SlurpWord()
		base.CString = false
	}

	if base.EOL() || base.RawDate == "" {
		return base, ErrorParsingDate
	}

	if base.ExpectRune('[') {
		// the context is first so assume the line remainder is the message
		if r, err := base.EnclosedString(']', false); err == nil {
			base.RawContext = r
		}

		for base.Expect(unicode.Space) {
			base.Next()
		}
	} else {
		// the context isn't first so there is likely more available to check
		for i := 0; i < 4; i += 1 {
			if part, ok := base.SlurpWord(); ok {
				if base.Severity == record.SeverityNone &&
					base.RawComponent == "" &&
					base.RawContext == "" {
					severity, ok := record.NewSeverity(part)

					if ok {
						base.Severity = severity
						continue
					}
				}
				if base.RawComponent == "" && record.IsComponent(part) {
					base.RawComponent = part
					continue
				} else if base.RawContext == "" && part[0] == '[' {
					base.RewindSlurpWord()
					if r, err := base.EnclosedString(']', false); err == nil {
						base.RawContext = r
						continue
					}
				}

				base.RewindSlurpWord()
				break
			}
		}
	}

	// All log entries for all supported versions have a context.
	if base.RawContext == "" {
		return base, ErrorMissingContext
	}

	pos = base.Pos()
	base.RawMessage = base.Remainder()
	base.Seek(pos, 0)

	return base, nil
}

func (f *Log) Close() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.closed {
		f.closed = true
		return f.Closer.Close()
	}

	return nil
}

func (f *Log) isClosed() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.closed
}

func (f *Log) Get() (record.Base, error) {
	return f.next, f.error
}

func (f *Log) Next() bool {
	f.next, f.error = f.get()

	if f.error == io.EOF {
		return false
	}
	return true
}

func (f Log) get() (record.Base, error) {
	if !f.eof && !f.isClosed() && f.Scanner.Scan() {
		f.line += 1
		return f.NewBase(f.Scanner.Text(), f.line)
	}
	return record.Base{}, io.EOF
}

// Take a parts array ([]string { "Sun", "Jan", "02", "15:04:05" }) and combined into a single element
// ([]string { "Sun Jan 02 15:04:05" }) with all trailing elements appended to the array.
func parseCDateString(r *record.Base) string {
	var (
		ok     = true
		target = make([]string, 4)
	)
	start := r.Pos()
	for i := 0; i < 4 && ok; i++ {
		target[i], ok = r.SlurpWord()
	}

	switch {
	case !internal.IsDay(target[0]):
	case !internal.IsMonth(target[1]):
	case !internal.IsNumeric(target[2]):
	case !internal.IsTime(target[3]):
		r.Seek(start, 0)
		return ""
	}

	return strings.Join(target, " ")
}
