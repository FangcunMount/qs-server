package retirement

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// strictJSON also rejects unpaired UTF-16 escapes: encoding/json otherwise
// replaces them, just as it silently replaces invalid UTF-8 input.
func strictJSON(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxSourceRowBytes*2 || !utf8.Valid(raw) {
		return ErrSourceJSON
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		i++
		for ; i < len(raw) && raw[i] != '"'; i++ {
			if raw[i] != '\\' {
				continue
			}
			i++
			if i >= len(raw) {
				return ErrSourceJSON
			}
			if raw[i] != 'u' {
				continue
			}
			if i+4 >= len(raw) {
				return ErrSourceJSON
			}
			code, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			if err != nil {
				return ErrSourceJSON
			}
			i += 4
			if code >= 0xD800 && code <= 0xDBFF {
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return ErrSourceJSON
				}
				low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
				if err != nil || low < 0xDC00 || low > 0xDFFF {
					return ErrSourceJSON
				}
				i += 6
			} else if code >= 0xDC00 && code <= 0xDFFF {
				return ErrSourceJSON
			}
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := strictJSONValue(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrSourceJSON
	}
	return nil
}

func strictJSONValue(d *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrSourceBounds
	}
	token, err := d.Token()
	if err != nil {
		return ErrSourceJSON
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return ErrSourceJSON
			}
			text, ok := key.(string)
			if !ok {
				return ErrSourceJSON
			}
			if keys[text] {
				return ErrSourceDuplicateKey
			}
			keys[text] = true
			if err := strictJSONValue(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return ErrSourceJSON
		}
	case '[':
		for d.More() {
			if err := strictJSONValue(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return ErrSourceJSON
		}
	default:
		return ErrSourceJSON
	}
	return nil
}

// Struct shape checking is case-sensitive and requires producer fields that
// have no omitempty. Go's normal case-insensitive/zero-value decoder is too
// permissive for historical evidence. Optional fields may be absent, not null.
func strictTyped(raw []byte, target any) error {
	if err := strictJSON(raw); err != nil {
		return err
	}
	t := reflect.TypeOf(target)
	if t == nil || t.Kind() != reflect.Pointer {
		return ErrSourceSchema
	}
	if err := typedShape(raw, t.Elem()); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return ErrSourceSchema
	}
	return nil
}

var sourceTimeType = reflect.TypeOf(time.Time{})
var sourceRawType = reflect.TypeOf(json.RawMessage{})

func typedShape(raw []byte, t reflect.Type) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ErrSourceSchema
	}
	if t.Kind() == reflect.Pointer {
		return typedShape(raw, t.Elem())
	}
	if t == sourceRawType {
		return nil
	}
	if t == sourceTimeType {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return ErrSourceSchema
		}
		clock, err := time.Parse(time.RFC3339Nano, text)
		if err != nil || clock.IsZero() {
			return ErrSourceSchema
		}
		return nil
	}
	if t.Kind() == reflect.Struct {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return ErrSourceSchema
		}
		allowed := map[string]reflect.StructField{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "" || name == "-" {
				continue
			}
			allowed[name] = f
			if !strings.Contains(tag, ",omitempty") {
				if _, ok := fields[name]; !ok {
					return ErrSourceSchema
				}
			}
		}
		for name, v := range fields {
			f, ok := allowed[name]
			if !ok {
				return ErrSourceUnknownField
			}
			if err := typedShape(v, f.Type); err != nil {
				return err
			}
		}
		return nil
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Uint, reflect.Uint64, reflect.Uint32:
		var number json.Number
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var v any
		if d.Decode(&v) != nil {
			return ErrSourceSchema
		}
		n, ok := v.(json.Number)
		if !ok {
			return ErrSourceSchema
		}
		number = n
		if t.Kind() == reflect.Uint || t.Kind() == reflect.Uint64 || t.Kind() == reflect.Uint32 {
			if _, err := strconv.ParseUint(number.String(), 10, t.Bits()); err != nil {
				return ErrSourcePrecision
			}
		} else {
			if _, err := strconv.ParseInt(number.String(), 10, t.Bits()); err != nil {
				return ErrSourcePrecision
			}
		}
	case reflect.Float64:
		var number json.Number
		if json.Unmarshal(raw, &number) != nil {
			return ErrSourceSchema
		}
		f, err := number.Float64()
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return ErrSourcePrecision
		}
		original, ok := new(big.Rat).SetString(number.String())
		rounded, rok := new(big.Rat).SetString(strconv.FormatFloat(f, 'g', -1, 64))
		if !ok || !rok || original.Cmp(rounded) != 0 {
			return ErrSourcePrecision
		}
	case reflect.String:
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return ErrSourceSchema
		}
	default:
		return ErrSourceSchema
	}
	return nil
}
