package metadata_test

import (
	"context"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/nostosx/servex/metadata"
)

func TestNew(t *testing.T) {
	md := metadata.New(map[string]string{"X-Request-ID": "abc", "Accept": "json"})
	want := metadata.MD{"x-request-id": {"abc"}, "accept": {"json"}}
	if !equalMD(md, want) {
		t.Fatalf("New = %v, want %v", md, want)
	}
	if got := metadata.New(nil); got == nil || len(got) != 0 {
		t.Fatalf("New(nil) = %#v, want empty non-nil MD", got)
	}
}

func TestPairs(t *testing.T) {
	tests := []struct {
		name string
		kv   []string
		want metadata.MD
	}{
		{name: "empty", kv: nil, want: metadata.MD{}},
		{name: "single", kv: []string{"Key", "v"}, want: metadata.MD{"key": {"v"}}},
		{
			name: "repeated keys accumulate in order",
			kv:   []string{"K", "1", "k", "2", "other", "x", "K", "3"},
			want: metadata.MD{"k": {"1", "2", "3"}, "other": {"x"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metadata.Pairs(tt.kv...); !equalMD(got, tt.want) {
				t.Fatalf("Pairs(%q) = %v, want %v", tt.kv, got, tt.want)
			}
		})
	}
}

func TestPairsOddPanics(t *testing.T) {
	for _, kv := range [][]string{{"a"}, {"a", "b", "c"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Pairs(%q) did not panic", kv)
				}
			}()
			metadata.Pairs(kv...)
		}()
	}
}

func TestJoin(t *testing.T) {
	a := metadata.Pairs("k", "1", "only-a", "a")
	b := metadata.MD{"K": {"2", "3"}, "only-b": {"b"}} // not normalized on purpose
	got := metadata.Join(a, nil, b)
	want := metadata.MD{"k": {"1", "2", "3"}, "only-a": {"a"}, "only-b": {"b"}}
	if !equalMD(got, want) {
		t.Fatalf("Join = %v, want %v", got, want)
	}

	// The result must not alias the inputs.
	got.Add("k", "4")
	got["only-a"][0] = "changed"
	if !slices.Equal(a["k"], []string{"1"}) || a.Get("only-a") != "a" {
		t.Fatalf("Join aliased its input: %v", a)
	}

	if empty := metadata.Join(); empty == nil || len(empty) != 0 {
		t.Fatalf("Join() = %#v, want empty non-nil MD", empty)
	}
}

func TestCaseInsensitiveAccess(t *testing.T) {
	md := metadata.MD{}
	md.Set("Content-Type", "text/plain")
	md.Add("CONTENT-TYPE", "application/json")
	md.Add("X-Trace", "t1")

	if got := md.Get("content-type"); got != "text/plain" {
		t.Errorf("Get = %q, want %q", got, "text/plain")
	}
	if got := md.Values("Content-Type"); !slices.Equal(got, []string{"text/plain", "application/json"}) {
		t.Errorf("Values = %q", got)
	}
	if _, ok := md["content-type"]; !ok {
		t.Errorf("stored key not lower case: %v", md)
	}

	md.Set("CONTENT-type", "x")
	if got := md.Values("content-type"); !slices.Equal(got, []string{"x"}) {
		t.Errorf("Set did not overwrite: %q", got)
	}

	md.Delete("X-TRACE")
	if got := md.Values("x-trace"); got != nil {
		t.Errorf("Delete left %q", got)
	}

	if got := md.Get("missing"); got != "" {
		t.Errorf("Get(missing) = %q, want empty", got)
	}
	if got := md.Values("missing"); got != nil {
		t.Errorf("Values(missing) = %q, want nil", got)
	}
}

func TestNilMDReads(t *testing.T) {
	var md metadata.MD
	if md.Get("a") != "" || md.Values("a") != nil || len(md.Keys()) != 0 {
		t.Fatal("reads on nil MD must return zero values")
	}
	md.Delete("a") // must not panic
	for range md.All() {
		t.Fatal("All on nil MD must yield nothing")
	}
}

func TestKeysAndAllSorted(t *testing.T) {
	md := metadata.Pairs("c", "3", "A", "1", "b", "2", "a", "1b")
	wantKeys := []string{"a", "b", "c"}
	if got := md.Keys(); !slices.Equal(got, wantKeys) {
		t.Fatalf("Keys = %q, want %q", got, wantKeys)
	}

	var keys []string
	for k, vs := range md.All() {
		keys = append(keys, k)
		if !slices.Equal(vs, md[k]) {
			t.Errorf("All yielded %q for %q, want %q", vs, k, md[k])
		}
	}
	if !slices.Equal(keys, wantKeys) {
		t.Fatalf("All keys = %q, want %q", keys, wantKeys)
	}

	// Early termination.
	var first []string
	for k := range md.All() {
		first = append(first, k)
		break
	}
	if !slices.Equal(first, []string{"a"}) {
		t.Fatalf("All with break = %q", first)
	}
}

func TestClone(t *testing.T) {
	var nilMD metadata.MD
	if nilMD.Clone() != nil {
		t.Fatal("Clone of nil MD must be nil")
	}

	orig := metadata.Pairs("k", "1", "k", "2")
	clone := orig.Clone()
	if !equalMD(orig, clone) {
		t.Fatalf("Clone = %v, want %v", clone, orig)
	}
	clone["k"][0] = "changed"
	clone.Add("k", "3")
	clone.Set("new", "x")
	if !equalMD(orig, metadata.Pairs("k", "1", "k", "2")) {
		t.Fatalf("Clone is not deep: original became %v", orig)
	}

	empty := metadata.MD{}.Clone()
	if empty == nil || len(empty) != 0 {
		t.Fatalf("Clone of empty MD = %#v, want empty non-nil", empty)
	}
}

func TestConcurrentReads(t *testing.T) {
	md := metadata.Pairs("a", "1", "b", "2", "c", "3")
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if md.Get("A") != "1" || len(md.Keys()) != 3 || len(md.Clone()) != 3 {
				t.Error("unexpected concurrent read result")
			}
		})
	}
	wg.Wait()
}

func TestInfoContext(t *testing.T) {
	if _, ok := metadata.FromContext(t.Context()); ok {
		t.Fatal("FromContext on empty context reported ok")
	}
	var nilCtx context.Context
	if _, ok := metadata.FromContext(nilCtx); ok {
		t.Fatal("FromContext(nil) reported ok")
	}

	info := metadata.Info{
		Kind:      metadata.KindServer,
		Protocol:  "http",
		Operation: "GET /v1/users/{id}",
		Header:    metadata.Pairs("x-request-id", "r1"),
	}
	ctx := metadata.NewContext(t.Context(), info)
	got, ok := metadata.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext did not find Info")
	}
	if got.Kind != info.Kind || got.Protocol != info.Protocol || got.Operation != info.Operation {
		t.Fatalf("FromContext = %+v, want %+v", got, info)
	}

	// The header is shared so middleware can write outbound metadata.
	got.Header.Set("x-added", "yes")
	if info.Header.Get("x-added") != "yes" {
		t.Fatal("Info.Header is expected to be shared through the context")
	}

	// Inner contexts shadow outer ones.
	inner := metadata.NewContext(ctx, metadata.Info{Kind: metadata.KindClient, Protocol: "grpc"})
	if got, _ := metadata.FromContext(inner); got.Kind != metadata.KindClient || got.Protocol != "grpc" {
		t.Fatalf("inner FromContext = %+v", got)
	}
	if got, _ := metadata.FromContext(ctx); got.Kind != metadata.KindServer {
		t.Fatalf("outer context changed: %+v", got)
	}
}

func TestKindValues(t *testing.T) {
	if metadata.KindServer != "server" || metadata.KindClient != "client" {
		t.Fatalf("unexpected Kind values %q %q", metadata.KindServer, metadata.KindClient)
	}
}

func equalMD(a, b metadata.MD) bool {
	return maps.EqualFunc(a, b, slices.Equal[[]string])
}
