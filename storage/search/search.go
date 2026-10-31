// Package search defines document indexing and text retrieval contracts.
package search

import "context"

// Document contains searchable text and exact metadata labels.
type Document struct {
	ID       string
	Text     string
	Metadata map[string]string
	Payload  []byte
}

// Query is a bounded text search. Metadata labels are combined with AND.
// Text is interpreted using the adapter's documented query syntax.
type Query struct {
	Text     string
	Metadata map[string]string
	Limit    int
	Offset   int
}

// Hit holds a result and its backend-specific score (larger is better).
// Scores must not be compared between different engines or queries.
type Hit struct {
	Document Document
	Score    float64
}

// Indexer atomically writes or deletes a batch of documents.
type Indexer interface {
	Upsert(context.Context, ...Document) error
	Delete(context.Context, ...string) error
}

// Searcher is safe for concurrent use, honors cancellation and returns detached
// results ordered by decreasing score, then ID for ties.
type Searcher interface {
	Search(context.Context, Query) ([]Hit, error)
}

// Index composes indexing and retrieval.
type Index interface {
	Indexer
	Searcher
}
