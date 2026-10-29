package encoding

// Codec marshals values to bytes and back.
type Codec interface {
	// Name returns the lower-case name of the codec, such as "json", "proto" or "yaml".
	Name() string
	// Marshal encodes v.
	Marshal(v any) ([]byte, error)
	// Unmarshal decodes data into v, which must be a non-nil pointer.
	Unmarshal(data []byte, v any) error
}
