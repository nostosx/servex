package archtest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/nostosx/servex/app"
	"github.com/nostosx/servex/discovery"
	discoverymemory "github.com/nostosx/servex/discovery/memory"
	"github.com/nostosx/servex/health"
	"github.com/nostosx/servex/messaging/broker"
	brokermemory "github.com/nostosx/servex/messaging/broker/memory"
	"github.com/nostosx/servex/messaging/jobqueue"
	jobqueuememory "github.com/nostosx/servex/messaging/jobqueue/memory"
	"github.com/nostosx/servex/metadata"
	"github.com/nostosx/servex/observability/tracing"
	"github.com/nostosx/servex/storage/cache"
	cachememory "github.com/nostosx/servex/storage/cache/memory"
	"github.com/nostosx/servex/storage/database"
	"github.com/nostosx/servex/transport"
	"github.com/nostosx/servex/transport/httpx"
)

// The foundation packages do not import each other; they meet through
// structural typing. These assertions pin the shapes they rely on.
var (
	_ tracing.Carrier = metadata.MD(nil)
	_ health.Checker  = (*app.App)(nil)
)

// Reference implementations satisfy the contracts they implement.
var (
	_ discovery.Registrar  = (*discoverymemory.Registry)(nil)
	_ discovery.Discoverer = (*discoverymemory.Registry)(nil)
	_ cache.Cache          = (*cachememory.Cache)(nil)
	_ broker.Publisher     = (*brokermemory.Broker)(nil)
	_ broker.Subscriber    = (*brokermemory.Broker)(nil)
	_ jobqueue.Enqueuer    = (*jobqueuememory.Queue)(nil)
	_ jobqueue.Runner      = (*jobqueuememory.Queue)(nil)
)

// The HTTP server is a transport.Server, an http.Handler, reports runtime
// failures through the optional Err method app watches, and can be handed
// to discovery.Registration.
var (
	_ transport.Server                = (*httpx.Server)(nil)
	_ http.Handler                    = (*httpx.Server)(nil)
	_ interface{ Err() <-chan error } = (*httpx.Server)(nil)
	_ interface{ URL() *url.URL }     = transport.Server(nil)
)

// TestOptionalHookMethods checks that the hooks returned by constructors
// implement the optional methods app discovers by type assertion.
func TestOptionalHookMethods(t *testing.T) {
	type errReporter interface{ Err() <-chan error }
	type checker interface {
		Check(ctx context.Context) error
	}

	db := sql.OpenDB(nopConnector{})
	t.Cleanup(func() { db.Close() })
	if _, ok := database.Hook("db", db).(checker); !ok {
		t.Error("database.Hook does not implement Check")
	}
	if _, ok := broker.NewConsumer(brokermemory.New(), "t", "", nopEndpoint).(errReporter); !ok {
		t.Error("broker.NewConsumer does not implement Err")
	}
	if _, ok := jobqueue.NewWorker(jobqueuememory.New(), nil).(errReporter); !ok {
		t.Error("jobqueue.NewWorker does not implement Err")
	}
	if _, ok := app.Hook(httpx.NewServer()).(errReporter); !ok {
		t.Error("httpx.Server does not implement Err")
	}
}

func nopEndpoint(context.Context, any) (any, error) { return nil, nil }

// nopConnector is a driver.Connector that never connects; the test only
// inspects method sets.
type nopConnector struct{}

func (nopConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("archtest: no connection")
}

func (nopConnector) Driver() driver.Driver { return nil }
