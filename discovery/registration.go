package discovery

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"uuid"

	"github.com/nostosx/servex/app"
	"github.com/nostosx/servex/internal/host"
)

// Registration returns an app.Hook named "registration:<Name>" that
// registers a copy of ins while the application runs.
//
// On Start it reads the URL of every server, replaces unspecified hosts
// (empty, 0.0.0.0 or ::) with an address of this machine, appends the
// resulting URLs to the copy's Endpoints and registers the copy, generating
// an ID with uuid.NewV7 when ins.ID is empty. Start fails when any server or
// URL is nil. The hook implements the optional app Drain method: when a
// running App stops, Drain deregisters the instance before the stop delay
// and before any server stops, so clients stop picking it while in-flight
// requests finish. Stop deregisters an instance that is still registered,
// which retries a failed Drain once and covers the rollback of a failed
// application start, where Drain is not called.
//
// Place the hook after the servers in the application so that their URLs
// are valid when it starts.
func Registration(r Registrar, ins Instance, servers ...interface{ URL() *url.URL }) app.Hook {
	return newRegistration(r, ins, servers, host.Extract)
}

// urlServer is the structural server type accepted by Registration.
type urlServer = interface{ URL() *url.URL }

// registration is the Hook returned by Registration.
type registration struct {
	r       Registrar
	ins     Instance
	servers []urlServer
	extract func(hostport string) (string, error)

	mu         sync.Mutex
	registered *Instance
}

// newRegistration builds a registration with an injectable host extractor.
func newRegistration(r Registrar, ins Instance, servers []urlServer, extract func(string) (string, error)) *registration {
	return &registration{
		r:       r,
		ins:     *ins.clone(),
		servers: servers,
		extract: extract,
	}
}

// Name implements app.Hook.
func (g *registration) Name() string { return "registration:" + g.ins.Name }

// Start implements app.Hook.
func (g *registration) Start(ctx context.Context) error {
	ins := g.ins.clone()
	for i, s := range g.servers {
		if s == nil {
			return fmt.Errorf("discovery: server %d is nil", i)
		}
		u := s.URL()
		if u == nil {
			return fmt.Errorf("discovery: server %d has a nil url", i)
		}
		ep, err := g.endpoint(u)
		if err != nil {
			return fmt.Errorf("discovery: server %d: %w", i, err)
		}
		ins.Endpoints = append(ins.Endpoints, ep)
	}
	if ins.ID == "" {
		ins.ID = uuid.NewV7().String()
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.r.Register(ctx, ins); err != nil {
		return fmt.Errorf("discovery: register %q: %w", ins.Name, err)
	}
	g.registered = ins
	return nil
}

// Drain deregisters the instance. After a failure the instance stays
// recorded as registered, so Stop tries once more.
func (g *registration) Drain(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.deregisterLocked(ctx)
}

// Stop implements app.Hook. It deregisters the instance unless Drain
// already did, and is a no-op when nothing is registered.
func (g *registration) Stop(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	ins := g.registered
	// Stop is the last attempt: forget the instance even if it fails.
	defer func() {
		if g.registered == ins {
			g.registered = nil
		}
	}()
	return g.deregisterLocked(ctx)
}

// deregisterLocked deregisters the registered instance, if any, and forgets
// it on success. g.mu must be held.
func (g *registration) deregisterLocked(ctx context.Context) error {
	ins := g.registered
	if ins == nil {
		return nil
	}
	if err := g.r.Deregister(ctx, ins); err != nil {
		return fmt.Errorf("discovery: deregister %q: %w", ins.Name, err)
	}
	g.registered = nil
	return nil
}

// endpoint converts a server URL into an advertisable endpoint string.
func (g *registration) endpoint(u *url.URL) (string, error) {
	u = u.Clone()
	h, err := g.extract(u.Host)
	if err != nil {
		return "", err
	}
	u.Host = h
	return u.String(), nil
}
