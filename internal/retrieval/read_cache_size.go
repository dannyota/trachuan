package retrieval

import (
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Count the JSON bytes without preparing a cache buffer. Stop once the value
// cannot fit; raw strings alone can rule out a large scope without scanning it.
func sequenceSize(seq readSequence, limit int64) int64 {
	stored := cachedSequence{Doc: seq.doc, Source: seq.source, Units: seq.units, Parts: seq.parts, Gaps: seq.gaps, Offset: seq.offset, More: seq.more, Complete: seq.complete}
	n := cacheJSONSize(reflect.ValueOf(stored), limit)
	if seq.document != nil && n <= limit {
		size, _ := cacheJSONFields(reflect.ValueOf(*seq.document), limit, "Status")
		n += int64(len(`,"Document":`)) + 2 + size
	}
	return n
}

// This counter supports only the immutable read-cache schema. Unknown kinds
// refuse caching rather than invoking arbitrary MarshalJSON implementations.
func cacheJSONSize(v reflect.Value, limit int64) int64 {
	if limit < 0 {
		return 0
	}
	if !v.IsValid() {
		return 4
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return 4
		}
		return cacheJSONSize(v.Elem(), limit)
	}
	if v.Type() == reflect.TypeFor[Date]() {
		return cacheJSONStringSize(v.Interface().(Date).String(), limit)
	}
	if v.Type() == reflect.TypeFor[time.Time]() {
		return cacheJSONStringSize(v.Interface().(time.Time).Format(time.RFC3339Nano), limit)
	}
	switch v.Kind() {
	case reflect.String:
		return cacheJSONStringSize(v.String(), limit)
	case reflect.Bool:
		if v.Bool() {
			return 4
		}
		return 5
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int64(len(strconv.FormatInt(v.Int(), 10)))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(len(strconv.FormatUint(v.Uint(), 10)))
	case reflect.Struct:
		n, _ := cacheJSONFields(v, limit, "")
		return n + 2
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return 4
		}
		// No byte slices are present here: cachedSequence.Document stays omitted
		// until its metadata size has passed the preflight above.
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return limit + 1
		}
		n := int64(2)
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				n++
			}
			n += cacheJSONSize(v.Index(i), limit-n)
			if n > limit {
				return n
			}
		}
		return n
	case reflect.Map:
		if v.IsNil() {
			return 4
		}
		n := int64(2)
		it := v.MapRange()
		first := true
		for it.Next() {
			if !first {
				n++
			}
			first = false
			key := it.Key()
			if key.Kind() == reflect.String {
				n += cacheJSONStringSize(key.String(), limit-n)
			} else if key.Kind() == reflect.Int {
				n += int64(len(strconv.FormatInt(key.Int(), 10)) + 2)
			} else {
				return limit + 1
			}
			n++
			n += cacheJSONSize(it.Value(), limit-n)
			if n > limit {
				return n
			}
		}
		return n
	default:
		return limit + 1
	}
}

func cacheJSONFields(v reflect.Value, limit int64, skip string) (int64, int) {
	n := int64(0)
	count := 0
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if field.PkgPath != "" || field.Name == skip {
			continue
		}
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		value := v.Field(i)
		if options == "omitempty" && cacheJSONEmpty(value) {
			continue
		}
		var size int64
		members := 1
		if field.Anonymous && name == "" {
			size, members = cacheJSONFields(value, limit-n, "")
		} else {
			if name == "" {
				name = field.Name
			}
			size = cacheJSONStringSize(name, limit-n) + 1
			size += cacheJSONSize(value, limit-n-size)
		}
		if members == 0 {
			continue
		}
		if count > 0 {
			n++
		}
		n += size
		count += members
		if n > limit {
			return n, count
		}
	}
	return n, count
}

func cacheJSONEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Pointer, reflect.Interface:
		return v.IsZero()
	default:
		return false
	}
}

func cacheJSONStringSize(s string, limit int64) int64 {
	if int64(len(s))+2 > limit {
		return limit + 1
	}
	n := int64(2)
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			i++
			switch c {
			case '"', '\\', '\b', '\f', '\n', '\r', '\t':
				n += 2
			case '<', '>', '&':
				n += 6
			default:
				if c < 0x20 {
					n += 6
				} else {
					n++
				}
			}
		} else {
			r, size := utf8.DecodeRuneInString(s[i:])
			i += size
			if r == utf8.RuneError && size == 1 {
				n += 3
			} else if r == '\u2028' || r == '\u2029' {
				n += 6
			} else {
				n += int64(size)
			}
		}
		if n > limit {
			return n
		}
	}
	return n
}
