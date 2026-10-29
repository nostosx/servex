package archtest

import (
	"net/url"

	"github.com/nostosx/servex/app"
	"github.com/nostosx/servex/health"
	"github.com/nostosx/servex/metadata"
	"github.com/nostosx/servex/observability/tracing"
	"github.com/nostosx/servex/transport"
)

var (
	_ tracing.Carrier             = metadata.MD(nil)
	_ health.Checker              = (*app.App)(nil)
	_ interface{ URL() *url.URL } = transport.Server(nil)
)
