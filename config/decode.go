package config

import (
	"cmp"
	"encoding"
	"errors"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	durationType        = reflect.TypeFor[time.Duration]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// decodeError reports a decoding failure at a configuration path.
type decodeError struct {
	path string
	err  error
}

func (e *decodeError) Error() string {
	path := e.path
	if path == "" {
		path = "<root>"
	}
	return "config: unmarshal " + strconv.Quote(path) + ": " + e.err.Error()
}

func (e *decodeError) Unwrap() error { return e.err }

// fail wraps err with the path it occurred at.
func fail(path string, err error) error { return &decodeError{path: path, err: err} }

// childPath appends a map key to a dotted path.
func childPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// indexPath appends a list index to a path.
func indexPath(path string, i int) string {
	return path + "[" + strconv.Itoa(i) + "]"
}

// decode stores the normalized value src into the settable value dst,
// applying weak type conversion.
func decode(path string, src any, dst reflect.Value) error {
	if src == nil {
		dst.SetZero()
		return nil
	}
	t := dst.Type()
	switch t.Kind() {
	case reflect.Pointer:
		if dst.IsNil() {
			dst.Set(reflect.New(t.Elem()))
		}
		return decode(path, src, dst.Elem())
	case reflect.Interface:
		if t.NumMethod() == 0 {
			dst.Set(reflect.ValueOf(clone(src)))
			return nil
		}
		if sv := reflect.ValueOf(src); sv.Type().AssignableTo(t) {
			dst.Set(sv)
			return nil
		}
		return fail(path, &convError{src, t.String()})
	case reflect.Map, reflect.Slice, reflect.Array:
		// Containers are always rebuilt so the tree is never aliased.
	default:
		if sv := reflect.ValueOf(src); sv.Type() == t {
			dst.Set(sv)
			return nil
		}
	}
	if s, ok := src.(string); ok && reflect.PointerTo(t).Implements(textUnmarshalerType) {
		tu := dst.Addr().Interface().(encoding.TextUnmarshaler)
		if err := tu.UnmarshalText([]byte(s)); err != nil {
			return fail(path, err)
		}
		return nil
	}
	if t == durationType {
		d, err := toDuration(src)
		if err != nil {
			return fail(path, err)
		}
		dst.SetInt(int64(d))
		return nil
	}
	switch t.Kind() {
	case reflect.Bool:
		b, err := toBool(src)
		if err != nil {
			return fail(path, err)
		}
		dst.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := toInt64(src)
		if err == nil && dst.OverflowInt(i) {
			err = fmt.Errorf("value %d overflows %s", i, t)
		}
		if err != nil {
			return fail(path, err)
		}
		dst.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, err := toUint64(src)
		if err == nil && dst.OverflowUint(u) {
			err = fmt.Errorf("value %d overflows %s", u, t)
		}
		if err != nil {
			return fail(path, err)
		}
		dst.SetUint(u)
	case reflect.Float32, reflect.Float64:
		f, err := toFloat64(src)
		if err == nil && dst.OverflowFloat(f) {
			err = fmt.Errorf("value %g overflows %s", f, t)
		}
		if err != nil {
			return fail(path, err)
		}
		dst.SetFloat(f)
	case reflect.String:
		s, err := toString(src)
		if err != nil {
			return fail(path, err)
		}
		dst.SetString(s)
	case reflect.Struct:
		return decodeStruct(path, src, dst)
	case reflect.Map:
		return decodeMap(path, src, dst)
	case reflect.Slice:
		return decodeSlice(path, src, dst)
	case reflect.Array:
		return decodeArray(path, src, dst)
	default:
		return fail(path, fmt.Errorf("unsupported destination type %s", t))
	}
	return nil
}

// decodeStruct decodes an object into the exported fields of a struct.
func decodeStruct(path string, src any, dst reflect.Value) error {
	m, ok := src.(map[string]any)
	if !ok {
		if sv := reflect.ValueOf(src); sv.Type().AssignableTo(dst.Type()) {
			dst.Set(sv)
			return nil
		}
		return fail(path, &convError{src, dst.Type().String()})
	}
	fields := structFields(dst.Type())
	for _, key := range slices.Sorted(maps.Keys(m)) {
		f, ok := matchField(fields, key)
		if !ok {
			continue
		}
		fp := childPath(path, key)
		fv, err := fieldByIndex(dst, f.index)
		if err != nil {
			return fail(fp, err)
		}
		if err := decode(fp, m[key], fv); err != nil {
			return err
		}
	}
	return nil
}

// matchField finds the field for key: an exact name match first, then the
// first field in declaration order whose name matches case-insensitively.
func matchField(fields []field, key string) (field, bool) {
	if i := slices.IndexFunc(fields, func(f field) bool { return f.name == key }); i >= 0 {
		return fields[i], true
	}
	if i := slices.IndexFunc(fields, func(f field) bool { return strings.EqualFold(f.name, key) }); i >= 0 {
		return fields[i], true
	}
	return field{}, false
}

// fieldByIndex returns the nested field at index, allocating nil embedded
// struct pointers on the way.
func fieldByIndex(v reflect.Value, index []int) (reflect.Value, error) {
	for i, x := range index {
		if i > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}, fmt.Errorf("cannot set embedded pointer to unexported struct %s", v.Type().Elem())
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, nil
}

// decodeMap decodes an object into a map, converting keys weakly. Existing
// entries are decoded into, so defaults survive for missing nested keys.
func decodeMap(path string, src any, dst reflect.Value) error {
	m, ok := src.(map[string]any)
	if !ok {
		return fail(path, &convError{src, dst.Type().String()})
	}
	t := dst.Type()
	if dst.IsNil() {
		dst.Set(reflect.MakeMapWithSize(t, len(m)))
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		ep := childPath(path, k)
		kv := reflect.New(t.Key()).Elem()
		if err := decode(ep, k, kv); err != nil {
			return err
		}
		ev := reflect.New(t.Elem()).Elem()
		if old := dst.MapIndex(kv); old.IsValid() {
			ev.Set(old)
		}
		if err := decode(ep, m[k], ev); err != nil {
			return err
		}
		dst.SetMapIndex(kv, ev)
	}
	return nil
}

// decodeSlice decodes a list into a freshly allocated slice. A non-list
// value becomes a single-element slice; a string decodes into []byte as its
// raw bytes.
func decodeSlice(path string, src any, dst reflect.Value) error {
	t := dst.Type()
	if s, ok := src.(string); ok && t.Elem().Kind() == reflect.Uint8 {
		dst.SetBytes([]byte(s))
		return nil
	}
	list, ok := src.([]any)
	if !ok {
		list = []any{src}
	}
	out := reflect.MakeSlice(t, len(list), len(list))
	for i, e := range list {
		if err := decode(indexPath(path, i), e, out.Index(i)); err != nil {
			return err
		}
	}
	dst.Set(out)
	return nil
}

// decodeArray decodes a list into a fixed-size array; missing trailing
// elements are zeroed and extra elements are an error.
func decodeArray(path string, src any, dst reflect.Value) error {
	list, ok := src.([]any)
	if !ok {
		list = []any{src}
	}
	if len(list) > dst.Len() {
		return fail(path, fmt.Errorf("list of %d elements does not fit %s", len(list), dst.Type()))
	}
	dst.SetZero()
	for i, e := range list {
		if err := decode(indexPath(path, i), e, dst.Index(i)); err != nil {
			return err
		}
	}
	return nil
}

// field describes a decodable struct field.
type field struct {
	name   string
	tagged bool
	index  []int
}

// structFields lists the decodable fields of struct type t, promoting the
// fields of embedded structs with the same dominance rules as encoding/json:
// the shallowest field wins, ties are broken by a json tag, and remaining
// ties hide the name entirely.
func structFields(t reflect.Type) []field {
	type level struct {
		typ   reflect.Type
		index []int
	}
	var all []field
	visited := map[reflect.Type]bool{}
	next := []level{{typ: t}}
	for len(next) > 0 {
		current := next
		next = nil
		for _, l := range current {
			if visited[l.typ] {
				continue
			}
			visited[l.typ] = true
			for sf := range l.typ.Fields() {
				tag := sf.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, _, _ := strings.Cut(tag, ",")
				index := slices.Concat(l.index, sf.Index)
				if sf.Anonymous && name == "" {
					et := sf.Type
					if et.Kind() == reflect.Pointer {
						et = et.Elem()
					}
					if et.Kind() == reflect.Struct {
						next = append(next, level{typ: et, index: index})
						continue
					}
				}
				if !sf.IsExported() {
					continue
				}
				all = append(all, field{name: cmp.Or(name, sf.Name), tagged: name != "", index: index})
			}
		}
	}
	slices.SortStableFunc(all, func(a, b field) int {
		return cmp.Or(
			strings.Compare(a.name, b.name),
			cmp.Compare(len(a.index), len(b.index)),
			compareTagged(a, b),
		)
	})
	var out []field
	for group := range chunkBy(all, func(f field) string { return f.name }) {
		if len(group) > 1 && len(group[0].index) == len(group[1].index) && group[0].tagged == group[1].tagged {
			continue
		}
		out = append(out, group[0])
	}
	slices.SortFunc(out, func(a, b field) int { return slices.Compare(a.index, b.index) })
	return out
}

// compareTagged orders tagged fields before untagged ones.
func compareTagged(a, b field) int {
	switch {
	case a.tagged == b.tagged:
		return 0
	case a.tagged:
		return -1
	}
	return 1
}

// chunkBy yields maximal runs of consecutive elements sharing the same key.
func chunkBy[E any, K comparable](s []E, key func(E) K) iter.Seq[[]E] {
	return func(yield func([]E) bool) {
		for start := 0; start < len(s); {
			end := start + 1
			for end < len(s) && key(s[end]) == key(s[start]) {
				end++
			}
			if !yield(s[start:end]) {
				return
			}
			start = end
		}
	}
}

// errNotPointer is returned by Unmarshal for invalid destinations.
var errNotPointer = errors.New("destination must be a non-nil pointer")
