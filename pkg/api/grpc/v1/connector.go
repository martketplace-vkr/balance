package v1

import (
	"context"
	"time"

	"github.com/martketplace-vkr/balance/pkg/api/grpc/v1/admin"
	"github.com/martketplace-vkr/balance/pkg/api/grpc/v1/client"
	"github.com/martketplace-vkr/balance/pkg/api/grpc/v1/order"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	cmpName = "CatalogClientGrpc"
)

type Connector struct {
	Admin  admin.BalanceAdminServiceClient
	Client client.BalanceClientServiceClient
	Order  order.BalanceOrderServiceClient

	conn *grpc.ClientConn
	cfg  Config
}

func New(cfg Config) *Connector {
	return &Connector{cfg: cfg}
}

func (c *Connector) Start(ctx context.Context) (err error) {
	if c.cfg.DontRun {
		return nil
	}

	options := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.FailOnNonTempDialError(true),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}

	if c.cfg.Retry != nil {
		options = append(options, grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  c.cfg.Retry.BaseDelay,
				Multiplier: c.cfg.Retry.Multiplier,
				Jitter:     c.cfg.Retry.Jitter,
				MaxDelay:   c.cfg.Retry.MaxDelay,
			},
		}))
	}

	c.conn, err = grpc.DialContext(
		ctx,
		c.cfg.Address,
		options...,
	)
	if err != nil {
		return err
	}

	c.Client = client.NewBalanceClientServiceClient(c.conn)
	c.Admin = admin.NewBalanceAdminServiceClient(c.conn)
	c.Order = order.NewBalanceOrderServiceClient(c.conn)

	return nil
}

func (c *Connector) Stop(_ context.Context) error {
	if c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func (c *Connector) GetStartTimeout() time.Duration {
	return c.cfg.StartTimeout.Duration
}

func (c *Connector) GetStopTimeout() time.Duration {
	return c.cfg.StopTimeout.Duration
}

func (c *Connector) GetShutdownDelay() time.Duration {
	return c.cfg.ShutdownDelay.Duration
}

func (c *Connector) GetName() string {
	return cmpName
}
