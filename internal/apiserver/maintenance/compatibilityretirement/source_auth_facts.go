package retirement

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"math"
	"reflect"
	"sort"
	"strconv"
	"time"
)

const privateFactsProtocol = "private-decoded-source-facts-v1"
const maxPrivateFactNodes = 100000
const maxPrivateFactBytes = 2 * MaxSourceRowBytes

// This fingerprint detects changes to the complete decoded DTO. It is neither
// the original byte digest, an SDK fingerprint nor a business-binding digest.
// Frames go directly to a hash; no private DTO or message body is serialized.
type privateFactHasher struct {
	h            hash.Hash
	bytes, nodes int
}

func privateFactsSHA(value any) ([32]byte, error) {
	h := &privateFactHasher{h: sha256.New()}
	if err := h.frame([]byte(privateFactsProtocol)); err != nil {
		return [32]byte{}, err
	}
	if err := h.value(reflect.ValueOf(value), 0); err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], h.h.Sum(nil))
	return result, nil
}

func (h *privateFactHasher) frame(raw []byte) error {
	if len(raw) > maxPrivateFactBytes-h.bytes-9 {
		return ErrSourceBounds
	}
	h.bytes += len(raw) + 9
	sourceFrame(h.h, raw, false)
	return nil
}

var privateTimeType = reflect.TypeOf(time.Time{})

func (h *privateFactHasher) value(v reflect.Value, depth int) error {
	h.nodes++
	if depth > 64 || h.nodes > maxPrivateFactNodes {
		return ErrSourceBounds
	}
	if !v.IsValid() {
		return h.frame([]byte("invalid"))
	}
	if err := h.frame([]byte(v.Type().PkgPath() + ":" + v.Type().String())); err != nil {
		return err
	}
	if v.Type() == privateTimeType {
		t := v.Interface().(time.Time)
		if err := h.frame([]byte(t.UTC().Format(time.RFC3339Nano))); err != nil {
			return err
		}
		name, offset := t.Zone()
		if err := h.frame([]byte(name)); err != nil {
			return err
		}
		return h.frame([]byte(strconv.Itoa(offset)))
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return h.frame([]byte("nil"))
		}
		if err := h.frame([]byte("present")); err != nil {
			return err
		}
		return h.value(v.Elem(), depth+1)
	case reflect.Bool:
		return h.frame([]byte(strconv.FormatBool(v.Bool())))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return h.frame([]byte(strconv.FormatInt(v.Int(), 10)))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return h.frame([]byte(strconv.FormatUint(v.Uint(), 10)))
	case reflect.Float32, reflect.Float64:
		n := v.Float()
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return ErrSourceAuthentication
		}
		var raw [8]byte
		binary.BigEndian.PutUint64(raw[:], math.Float64bits(n))
		return h.frame(raw[:])
	case reflect.String:
		return h.frame([]byte(v.String()))
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if f.PkgPath != "" {
				return ErrSourceAuthentication
			}
			if err := h.frame([]byte(f.Name)); err != nil {
				return err
			}
			if err := h.value(v.Field(i), depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Array, reflect.Slice:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return h.frame([]byte("nil"))
		}
		if err := h.frame([]byte(strconv.Itoa(v.Len()))); err != nil {
			return err
		}
		for i := 0; i < v.Len(); i++ {
			if err := h.value(v.Index(i), depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if v.IsNil() {
			return h.frame([]byte("nil"))
		}
		if v.Type().Key().Kind() != reflect.String {
			return ErrSourceAuthentication
		}
		if v.Len() > maxPrivateFactNodes-h.nodes {
			return ErrSourceBounds
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		if err := h.frame([]byte(strconv.Itoa(len(keys)))); err != nil {
			return err
		}
		for _, k := range keys {
			if err := h.value(k, depth+1); err != nil {
				return err
			}
			if err := h.value(v.MapIndex(k), depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return ErrSourceAuthentication
	}
}

// A returned fact snapshot is detached from the caller's editable source.
// time.Time is an immutable value; its private location/clock internals are not
// traversed. No JSON/BSON marshal hooks can expose or normalize source facts.
func clonePrivateFacts(v reflect.Value, depth int, nodes *int) (reflect.Value, error) {
	*nodes++
	if depth > 64 || *nodes > maxPrivateFactNodes {
		return reflect.Value{}, ErrSourceBounds
	}
	if !v.IsValid() {
		return reflect.Value{}, ErrSourceAuthentication
	}
	if v.Type() == privateTimeType {
		return reflect.ValueOf(v.Interface()), nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		inner, err := clonePrivateFacts(v.Elem(), depth+1, nodes)
		if err != nil {
			return reflect.Value{}, err
		}
		if v.Kind() == reflect.Pointer {
			out := reflect.New(v.Type().Elem())
			out.Elem().Set(inner)
			return out, nil
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(inner)
		return out, nil
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath != "" {
				return reflect.Value{}, ErrSourceAuthentication
			}
			child, err := clonePrivateFacts(v.Field(i), depth+1, nodes)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Field(i).Set(child)
		}
		return out, nil
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		var out reflect.Value
		if v.Kind() == reflect.Slice {
			out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		} else {
			out = reflect.New(v.Type()).Elem()
		}
		for i := 0; i < v.Len(); i++ {
			child, err := clonePrivateFacts(v.Index(i), depth+1, nodes)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Index(i).Set(child)
		}
		return out, nil
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		if v.Type().Key().Kind() != reflect.String || v.Len() > maxPrivateFactNodes-*nodes {
			return reflect.Value{}, ErrSourceBounds
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			child, err := clonePrivateFacts(iter.Value(), depth+1, nodes)
			if err != nil {
				return reflect.Value{}, err
			}
			out.SetMapIndex(iter.Key(), child)
		}
		return out, nil
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return v, nil
	default:
		return reflect.Value{}, ErrSourceAuthentication
	}
}
