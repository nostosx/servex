package discovery

import (
	"context"
	"maps"
	"slices"
)

// Instance describes one running instance of a service.
type Instance struct {
	// ID uniquely identifies the instance within its service.
	ID string
	// Name is the service name.
	Name string
	// Version is the service version.
	Version string
	// Weight is the relative selection weight of the instance, copied to the
	// selector nodes built from it. Zero means the selector's default weight.
	Weight int
	// Metadata holds arbitrary instance attributes.
	Metadata map[string]string
	// Endpoints lists the URLs the instance serves on, such as
	// "http://10.0.0.1:8000" or "grpc://10.0.0.1:9000".
	Endpoints []string
}

// clone returns a deep copy of ins.
func (ins *Instance) clone() *Instance {
	return &Instance{
		ID:        ins.ID,
		Name:      ins.Name,
		Version:   ins.Version,
		Weight:    ins.Weight,
		Metadata:  maps.Clone(ins.Metadata),
		Endpoints: slices.Clone(ins.Endpoints),
	}
}

// Registrar registers and deregisters service instances.
type Registrar interface {
	// Register makes ins discoverable.
	Register(ctx context.Context, ins *Instance) error
	// Deregister removes ins from the registry.
	Deregister(ctx context.Context, ins *Instance) error
}

// Discoverer watches the instances of a service.
type Discoverer interface {
	// Watch returns a Watcher for the service name. The first call to Next
	// returns the current full instance list (possibly empty) immediately;
	// each later call returns the new full list after a change.
	//
	// ctx bounds only the Watch call itself. The returned Watcher lives until
	// its Stop is called; implementations must not tie it to ctx.
	Watch(ctx context.Context, name string) (Watcher, error)
}

// Watcher delivers successive snapshots of a service's instances.
type Watcher interface {
	// Next blocks until a new snapshot is available and returns it. It
	// returns an error once the watcher is stopped.
	Next() ([]*Instance, error)
	// Stop stops the watcher and unblocks pending Next calls.
	Stop() error
}
