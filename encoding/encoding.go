package encoding

import "encoding/json/v2"

// Codec marshals values to bytes and back.
type Codec interface {
	// Name returns the lower-case name of the codec, such as "json", "proto" or "yaml".
	Name() string
	// Marshal encodes v.
	Marshal(v any) ([]byte, error)
	// Unmarshal decodes data into v, which must be a non-nil pointer.
	Unmarshal(data []byte, v any) error
}

// JSON is a Codec named "json" backed by encoding/json/v2 with its default
// semantics. It is safe for concurrent use and must not be reassigned.
var JSON Codec = jsonCodec{}

// jsonCodec implements Codec using encoding/json/v2.
type jsonCodec struct{}

// Name returns "json".
func (jsonCodec) Name() string { return "json" }

// Marshal encodes v with json.Marshal.
func (jsonCodec) Marshal(v any) ([]byte, error) { return json.Marshal(v) }

// Unmarshal decodes data into v with json.Unmarshal.
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
