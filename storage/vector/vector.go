// Package vector defines backend-independent vector storage and retrieval.
// Embedding generation, index construction and execution policies belong to adapters.
package vector

import "context"

// Record is an application-owned object. Payload is opaque to the backend.
// Metadata contains exact string labels; it is not a query language.
type Record struct {
	ID       string
	Vector   []float32
	Metadata map[string]string
	Payload  []byte
}

// Query searches one store's namespace. All metadata labels must match.
type Query struct {
	Vector   []float32
	Metadata map[string]string
	// Limit must be positive. Implementations may impose a documented upper bound.
	Limit int
	// Approximate explicitly allows an index to trade recall for performance.
	// False requires exhaustive ranking even when ANN indexes exist.
	Approximate bool
}

// Hit reports a backend's nonnegative distance: smaller is closer.
// Distances are comparable only for the same metric and embedding model.
type Hit struct {
	Record   Record
	Distance float64
}

// Writer atomically writes each batch.
type Writer interface {
	Upsert(context.Context, ...Record) error
	Delete(context.Context, ...string) error
}

// Searcher returns hits ordered by increasing distance. Exact ties are ordered by
// ID in exhaustive mode. Approximate mode may return fewer than Limit hits.
// Implementations are concurrency-safe, honor cancellation, and do not generate embeddings.
type Searcher interface {
	Search(context.Context, Query) ([]Hit, error)
}

// Store composes independent write and search capabilities.
type Store interface {
	Writer
	Searcher
}
