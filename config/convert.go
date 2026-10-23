package config

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// convError reports that a value cannot be converted to a target type.
type convError struct {
	value  any
	target string
}

func (e *convError) Error() string {
	return "cannot convert " + describe(e.value) + " to " + e.target
}

// describe formats a value for error messages.
func describe(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return "string " + strconv.Quote(x)
	case map[string]any:
		return "object"
	case []any:
		return "list"
	}
	return fmt.Sprintf("%T %v", v, v)
}

// toString converts a scalar to its string form.
func toString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case time.Duration:
		return x.String(), nil
	case []byte:
		return string(x), nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return rv.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(rv.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'g', -1, rv.Type().Bits()), nil
	}
	return "", &convError{v, "string"}
}

// toInt64 converts integers, integral floats and decimal strings to int64.
func toInt64(v any) (int64, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if u := rv.Uint(); u <= math.MaxInt64 {
			return int64(u), nil
		}
	case reflect.Float32, reflect.Float64:
		if i, ok := floatToInt64(rv.Float()); ok {
			return i, nil
		}
	case reflect.String:
		s := strings.TrimSpace(rv.String())
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i, nil
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			if i, ok := floatToInt64(f); ok {
				return i, nil
			}
		}
	}
	return 0, &convError{v, "int"}
}

// floatToInt64 converts f when it is integral and within the int64 range.
func floatToInt64(f float64) (int64, bool) {
	if f != math.Trunc(f) || f < math.MinInt64 || f >= math.MaxInt64 {
		return 0, false
	}
	return int64(f), true
}

// toUint64 converts non-negative integers, integral floats and decimal
// strings to uint64.
func toUint64(v any) (uint64, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if i := rv.Int(); i >= 0 {
			return uint64(i), nil
		}
	case reflect.Float32, reflect.Float64:
		if u, ok := floatToUint64(rv.Float()); ok {
			return u, nil
		}
	case reflect.String:
		s := strings.TrimSpace(rv.String())
		if u, err := strconv.ParseUint(s, 10, 64); err == nil {
			return u, nil
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			if u, ok := floatToUint64(f); ok {
				return u, nil
			}
		}
	}
	return 0, &convError{v, "uint"}
}

// floatToUint64 converts f when it is integral and within the uint64 range.
func floatToUint64(f float64) (uint64, bool) {
	if f != math.Trunc(f) || f < 0 || f >= math.MaxUint64 {
		return 0, false
	}
	return uint64(f), true
}

// toFloat64 converts numbers and numeric strings to float64.
func toFloat64(v any) (float64, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(rv.Uint()), nil
	case reflect.String:
		if f, err := strconv.ParseFloat(strings.TrimSpace(rv.String()), 64); err == nil {
			return f, nil
		}
	}
	return 0, &convError{v, "float"}
}

// toBool converts booleans and strings accepted by strconv.ParseBool.
func toBool(v any) (bool, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool(), nil
	case reflect.String:
		if b, err := strconv.ParseBool(strings.TrimSpace(rv.String())); err == nil {
			return b, nil
		}
	}
	return false, &convError{v, "bool"}
}

// toDuration converts duration strings such as "5s", integer strings and
// numbers (interpreted as nanoseconds) to time.Duration.
func toDuration(v any) (time.Duration, error) {
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if d, err := time.ParseDuration(s); err == nil {
			return d, nil
		}
	}
	if i, err := toInt64(v); err == nil {
		return time.Duration(i), nil
	}
	return 0, &convError{v, "time.Duration"}
}
