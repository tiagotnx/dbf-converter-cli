// Package dbf implements a streaming reader for dBase III/IV .dbf files.
//
// It covers the common field types encountered in legacy business data:
// C (character), N / F (numeric), D (date), L (logical), I (int32), M (memo, as text id).
// Every value returned is AI-ready: text is trimmed and decoded to UTF-8,
// numerics are float64, dates are ISO-8601 strings or nil, and deleted records
// can be transparently skipped.
package dbf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// Field describes a single column declared in the DBF header.
type Field struct {
	Name    string
	Type    byte
	Length  int
	Decimal int
}

// Record represents one decoded row with AI-ready values.
// Values are keyed by field name and may be: string, float64, bool, or nil.
type Record struct {
	Values  map[string]interface{}
	Deleted bool
}

// Reader streams records out of a .dbf file.
type Reader struct {
	src           io.Reader
	decoder       *encoding.Decoder
	fields        []Field
	numRecords    uint32
	recordLen     int
	readRecords   uint32
	buf           []byte
	IgnoreDeleted bool
}

// NewReader parses the DBF header and prepares the stream for record iteration.
// encodingName selects the codepage used to decode character fields. "auto"
// uses byte 29 (the language-driver code) of the header to pick the codepage;
// supported names are cp850, windows-1252, iso-8859-1, utf-8.
func NewReader(src io.Reader, encodingName string) (*Reader, error) {
	// DBF header is 32 bytes.
	header := make([]byte, 32)
	if _, err := io.ReadFull(src, header); err != nil {
		return nil, fmt.Errorf("reading dbf header: %w", err)
	}

	resolvedName := encodingName
	if strings.EqualFold(encodingName, "auto") || encodingName == "" {
		resolvedName = detectEncoding(header[29])
	}
	dec, err := resolveDecoder(resolvedName)
	if err != nil {
		return nil, err
	}

	numRecords := binary.LittleEndian.Uint32(header[4:8])
	headerLen := int(binary.LittleEndian.Uint16(header[8:10]))
	recordLen := int(binary.LittleEndian.Uint16(header[10:12]))

	// Field descriptor area spans from byte 32 up to the 0x0D terminator.
	// dBase variants may pad between the terminator and the first record, so we
	// scan for 0x0D rather than computing field count from headerLen - 33.
	if headerLen < 33 {
		return nil, fmt.Errorf("invalid dbf header: declared length %d too small", headerLen)
	}
	rest := make([]byte, headerLen-32)
	if _, err := io.ReadFull(src, rest); err != nil {
		return nil, fmt.Errorf("reading dbf header body: %w", err)
	}

	// Walk the descriptor table in 32-byte strides. The first stride whose
	// leading byte is 0x0D marks the terminator; earlier strides are field
	// descriptors. This tolerates trailing pad bytes between terminator and
	// first record without conflating them with field descriptors.
	termIdx := -1
	for i := 0; i < len(rest); i += 32 {
		if rest[i] == 0x0D {
			termIdx = i
			break
		}
		if i+32 > len(rest) {
			break
		}
	}
	if termIdx == -1 {
		return nil, fmt.Errorf("invalid dbf header: 0x0D terminator not found within %d bytes", len(rest))
	}

	numFields := termIdx / 32
	fields := make([]Field, 0, numFields)
	for i := 0; i < numFields; i++ {
		descBuf := rest[i*32 : i*32+32]
		name := strings.TrimRight(string(descBuf[0:11]), "\x00 ")
		fields = append(fields, Field{
			Name:    name,
			Type:    descBuf[11],
			Length:  int(descBuf[16]),
			Decimal: int(descBuf[17]),
		})
	}

	return &Reader{
		src:           src,
		decoder:       dec,
		fields:        fields,
		numRecords:    numRecords,
		recordLen:     recordLen,
		buf:           make([]byte, recordLen),
		IgnoreDeleted: true,
	}, nil
}

// Fields returns the column definitions declared in the header.
func (r *Reader) Fields() []Field { return r.fields }

// NumRecords returns the record count declared by the header (includes deleted).
func (r *Reader) NumRecords() uint32 { return r.numRecords }

// Next returns the next record or (nil, nil) at EOF.
// When IgnoreDeleted is true (default), deleted records are transparently skipped.
func (r *Reader) Next() (*Record, error) {
	for {
		if r.readRecords >= r.numRecords {
			return nil, nil
		}
		if _, err := io.ReadFull(r.src, r.buf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, nil
			}
			return nil, fmt.Errorf("reading record %d: %w", r.readRecords, err)
		}
		r.readRecords++

		deleted := r.buf[0] == '*'
		if deleted && r.IgnoreDeleted {
			continue
		}

		values, err := r.decodeRecord(r.buf[1:])
		if err != nil {
			return nil, err
		}
		return &Record{Values: values, Deleted: deleted}, nil
	}
}

func (r *Reader) decodeRecord(raw []byte) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(r.fields))
	offset := 0
	for _, f := range r.fields {
		if offset+f.Length > len(raw) {
			return nil, fmt.Errorf("field %s: truncated record", f.Name)
		}
		fieldBytes := raw[offset : offset+f.Length]
		offset += f.Length

		val, err := r.decodeField(f, fieldBytes)
		if err != nil {
			return nil, fmt.Errorf("decoding field %s: %w", f.Name, err)
		}
		out[f.Name] = val
	}
	return out, nil
}

func (r *Reader) decodeField(f Field, raw []byte) (interface{}, error) {
	switch f.Type {
	case 'C', 'M':
		return r.decodeText(raw)
	case 'N', 'F':
		return decodeNumeric(raw)
	case 'D':
		return decodeDate(raw), nil
	case 'L':
		return decodeLogical(raw), nil
	case 'I':
		// 4-byte little-endian signed integer.
		if len(raw) < 4 {
			return nil, nil
		}
		return float64(int32(binary.LittleEndian.Uint32(raw[:4]))), nil
	default:
		// Unknown type: surface trimmed/decoded text as a safe fallback.
		return r.decodeText(raw)
	}
}

func (r *Reader) decodeText(raw []byte) (interface{}, error) {
	decoded, _, err := transform.Bytes(r.decoder, raw)
	if err != nil {
		return nil, fmt.Errorf("decoding text: %w", err)
	}
	return strings.TrimSpace(string(decoded)), nil
}

func decodeNumeric(raw []byte) (interface{}, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "." || s == "-" || strings.Trim(s, "*") == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, nil // stay permissive: unparseable numerics become nil
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, nil
	}
	return v, nil
}

// decodeDate parses YYYYMMDD and returns "YYYY-MM-DD" or nil for empty/invalid inputs.
func decodeDate(raw []byte) interface{} {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil
	}
	if len(s) != 8 {
		return nil
	}
	y, err := strconv.Atoi(s[0:4])
	if err != nil || y < 1 {
		return nil
	}
	m, err := strconv.Atoi(s[4:6])
	if err != nil || m < 1 || m > 12 {
		return nil
	}
	d, err := strconv.Atoi(s[6:8])
	if err != nil || d < 1 || d > 31 {
		return nil
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d)
}

func decodeLogical(raw []byte) interface{} {
	if len(raw) == 0 {
		return nil
	}
	switch raw[0] {
	case 'T', 't', 'Y', 'y':
		return true
	case 'F', 'f', 'N', 'n':
		return false
	default:
		return nil
	}
}

func resolveDecoder(name string) (*encoding.Decoder, error) {
	switch strings.ToLower(strings.ReplaceAll(name, "-", "")) {
	case "cp850", "ibm850":
		return charmap.CodePage850.NewDecoder(), nil
	case "windows1252", "cp1252":
		return charmap.Windows1252.NewDecoder(), nil
	case "iso88591", "latin1":
		return charmap.ISO8859_1.NewDecoder(), nil
	case "utf8":
		return encoding.Nop.NewDecoder(), nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %q (supported: auto, cp850, windows-1252, iso-8859-1, utf-8)", name)
	}
}

// detectEncoding maps the DBF language-driver code (header byte 29) to the
// closest codepage we support. When the byte is 0x00 (no declaration — the
// most common case for legacy Clipper/dBase files from the Brazilian ERP
// world) we fall back to CP850, the DOS codepage those tools wrote by default.
// Reference: https://www.dbase.com/Knowledgebase/INT/db7_file_fmt.htm
func detectEncoding(code byte) string {
	switch code {
	case 0x01, 0x02, 0x04, 0x69, 0x6A:
		// 0x01 = CP437 (US DOS) → closest supported is cp850
		// 0x02 = CP850, 0x04 = Macintosh Roman (map to cp850 fallback)
		return "cp850"
	case 0x03, 0x57, 0x58, 0x87, 0x88, 0x89:
		return "windows-1252"
	default:
		// 0x00 (absent) and all others default to cp850 — covers Clipper,
		// dBase III/IV and FoxBase+ sources, which are the primary audience.
		return "cp850"
	}
}
