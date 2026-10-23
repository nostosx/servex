package discovery

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"uuid"

	"github.com/nostosx/servex/app"
)

// fakeRegistrar records Register and Deregister calls.
type fakeRegistrar struct {
	mu            sync.Mutex
	registered    []*Instance
	deregistered  []*Instance
	registerErr   error
	deregisterErr error
}

func (f *fakeRegistrar) Register(_ context.Context, ins *Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.registerErr != nil {
		return f.registerErr
	}
	f.registered = append(f.registered, ins)
	return nil
}

func (f *fakeRegistrar) Deregister(_ context.Context, ins *Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deregisterErr != nil {
		return f.deregisterErr
	}
	f.deregistered = append(f.deregistered, ins)
	return nil
}

// fakeServer exposes a fixed URL.
type fakeServer struct{ u *url.URL }

func (s fakeServer) URL() *url.URL { return s.u }

func server(t *testing.T, raw string) fakeServer {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return fakeServer{u: u}
}

// fakeExtract replaces unspecified hosts with 10.9.9.9.
func fakeExtract(hostport string) (string, error) {
	if after, ok := strings.CutPrefix(hostport, "0.0.0.0:"); ok {
		return "10.9.9.9:" + after, nil
	}
	if after, ok := strings.CutPrefix(hostport, ":"); ok {
		return "10.9.9.9:" + after, nil
	}
	if after, ok := strings.CutPrefix(hostport, "[::]:"); ok {
		return "10.9.9.9:" + after, nil
	}
	return hostport, nil
}

func TestRegistrationName(t *testing.T) {
	h := Registration(&fakeRegistrar{}, Instance{Name: "user"})
	if got := h.Name(); got != "registration:user" {
		t.Fatalf("Name = %q, want registration:user", got)
	}
}

func TestRegistrationLifecycle(t *testing.T) {
	reg := &fakeRegistrar{}
	ins := Instance{
		ID:        "id-1",
		Name:      "user",
		Version:   "v1",
		Metadata:  map[string]string{"zone": "a"},
		Endpoints: []string{"static://example:1"},
	}
	servers := []urlServer{
		server(t, "http://0.0.0.0:8000"),
		server(t, "grpc://[::]:9000"),
		server(t, "http://:8080/base"),
		server(t, "http://10.0.0.1:7000"),
	}
	h := newRegistration(reg, ins, servers, fakeExtract)

	// Mutating the caller's instance after construction must not leak in.
	ins.Metadata["zone"] = "mutated"
	ins.Endpoints[0] = "mutated"

	if err := h.Start(t.Context()); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	if len(reg.registered) != 1 {
		t.Fatalf("registered %d instances, want 1", len(reg.registered))
	}
	got := reg.registered[0]
	want := []string{
		"static://example:1",
		"http://10.9.9.9:8000",
		"grpc://10.9.9.9:9000",
		"http://10.9.9.9:8080/base",
		"http://10.0.0.1:7000",
	}
	if !slices.Equal(got.Endpoints, want) {
		t.Errorf("Endpoints = %q, want %q", got.Endpoints, want)
	}
	if got.ID != "id-1" || got.Name != "user" || got.Version != "v1" {
		t.Errorf("identity = %q %q %q, want id-1 user v1", got.ID, got.Name, got.Version)
	}
	if got.Metadata["zone"] != "a" {
		t.Errorf("Metadata[zone] = %q, want a", got.Metadata["zone"])
	}
	// Server URLs must not be modified in place.
	if u := servers[0].URL(); u.Host != "0.0.0.0:8000" {
		t.Errorf("server URL mutated to %q", u.Host)
	}

	if err := h.Stop(t.Context()); err != nil {
		t.Fatalf("Stop error = %v", err)
	}
	if len(reg.deregistered) != 1 || reg.deregistered[0] != got {
		t.Fatalf("deregistered %v, want the registered instance", reg.deregistered)
	}
	// A second Stop is a no-op.
	if err := h.Stop(t.Context()); err != nil {
		t.Fatalf("second Stop error = %v", err)
	}
	if len(reg.deregistered) != 1 {
		t.Fatalf("deregistered %d times, want 1", len(reg.deregistered))
	}
}

func TestRegistrationGeneratesID(t *testing.T) {
	reg := &fakeRegistrar{}
	h := newRegistration(reg, Instance{Name: "user"}, []urlServer{server(t, "http://:80")}, fakeExtract)
	if err := h.Start(t.Context()); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	id := reg.registered[0].ID
	u, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("ID %q is not a UUID: %v", id, err)
	}
	// The version is the high nibble of byte 6.
	if v := u[6] >> 4; v != 7 {
		t.Fatalf("UUID version = %d, want 7", v)
	}
}

func TestRegistrationStartErrors(t *testing.T) {
	regErr := errors.New("registry down")
	extractErr := errors.New("bad host")
	tests := []struct {
		name    string
		servers []urlServer
		extract func(string) (string, error)
		regErr  error
		want    error
	}{
		{"nil url", []urlServer{server(t, "http://:80"), fakeServer{}}, fakeExtract, nil, nil},
		{"nil server", []urlServer{nil}, fakeExtract, nil, nil},
		{"extract failure", []urlServer{server(t, "http://:80")}, func(string) (string, error) { return "", extractErr }, nil, extractErr},
		{"register failure", []urlServer{server(t, "http://:80")}, fakeExtract, regErr, regErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &fakeRegistrar{registerErr: tt.regErr}
			h := newRegistration(reg, Instance{Name: "user"}, tt.servers, tt.extract)
			err := h.Start(t.Context())
			if err == nil {
				t.Fatal("Start error = nil, want error")
			}
			if !strings.HasPrefix(err.Error(), "discovery: ") {
				t.Errorf("error %q lacks the package prefix", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("Start error = %v, want %v", err, tt.want)
			}
			if len(reg.registered) != 0 {
				t.Errorf("registered %d instances, want 0", len(reg.registered))
			}
			// Stop after a failed Start does nothing.
			if err := h.Stop(t.Context()); err != nil {
				t.Errorf("Stop error = %v", err)
			}
			if len(reg.deregistered) != 0 {
				t.Errorf("deregistered %d instances, want 0", len(reg.deregistered))
			}
		})
	}
}

func TestRegistrationStopError(t *testing.T) {
	deregErr := errors.New("registry down")
	reg := &fakeRegistrar{deregisterErr: deregErr}
	h := newRegistration(reg, Instance{Name: "user"}, nil, fakeExtract)
	if err := h.Start(t.Context()); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	if err := h.Stop(t.Context()); !errors.Is(err, deregErr) {
		t.Fatalf("Stop error = %v, want %v", err, deregErr)
	}
}

func TestRegistrationStopWithoutStart(t *testing.T) {
	reg := &fakeRegistrar{}
	if err := Registration(reg, Instance{Name: "user"}).Stop(t.Context()); err != nil {
		t.Fatalf("Stop error = %v", err)
	}
	if len(reg.deregistered) != 0 {
		t.Fatal("Stop without Start must not deregister")
	}
}

// TestRegistrationRealHost runs Registration with the real host extractor.
// Specified hosts pass through unchanged on every machine.
func TestRegistrationRealHost(t *testing.T) {
	reg := &fakeRegistrar{}
	h := Registration(reg, Instance{ID: "x", Name: "user"}, server(t, "http://10.0.0.1:8000"))
	if err := h.Start(t.Context()); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	if got := reg.registered[0].Endpoints; !slices.Equal(got, []string{"http://10.0.0.1:8000"}) {
		t.Fatalf("Endpoints = %q", got)
	}
}

// TestRegistrationInApp checks that the hook takes part in an app lifecycle.
func TestRegistrationInApp(t *testing.T) {
	reg := &fakeRegistrar{}
	a := app.New(app.WithHooks(newRegistration(reg, Instance{Name: "user"}, []urlServer{server(t, "http://:80")}, fakeExtract)))
	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	if err := a.Stop(t.Context()); err != nil {
		t.Fatalf("Stop error = %v", err)
	}
	if len(reg.registered) != 1 || len(reg.deregistered) != 1 {
		t.Fatalf("registered %d, deregistered %d, want 1 and 1", len(reg.registered), len(reg.deregistered))
	}
}
