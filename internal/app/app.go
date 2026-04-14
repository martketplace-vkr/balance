package app

import (
	"context"

	// trmsqlx "github.com/avito-tech/go-transaction-manager/sqlx"
	// txmanager "github.com/avito-tech/go-transaction-manager/trm/manager"

	"github.com/martketplace-vkr/balance/config"
	"github.com/martketplace-vkr/balance/internal/app/cmp/server"
	repository "github.com/martketplace-vkr/balance/internal/repository/pg"
	servicebalance "github.com/martketplace-vkr/balance/internal/service/balance"
	adminTransport "github.com/martketplace-vkr/balance/internal/transport/grpc/v1/admin"
	clientTransport "github.com/martketplace-vkr/balance/internal/transport/grpc/v1/client"
	orderTransport "github.com/martketplace-vkr/balance/internal/transport/grpc/v1/order"

	"github.com/martketplace-vkr/pkg/build"
	"github.com/martketplace-vkr/pkg/build/components/pgxsqlxcomponent"
)

func Run(ctx context.Context, cfg *config.Config) error {
	pg := pgxsqlxcomponent.New(cfg.Postgres)

	// txManager, err := txmanager.New(trmsqlx.NewDefaultFactory(pg.DB))
	// if err != nil {
	// 	return err
	// }

	repo := repository.New(pg.DB)
	service := servicebalance.New(repo)
	adminHandler := adminTransport.New(service)
	clientHandler := clientTransport.New(service)
	orderHandler := orderTransport.New(service)

	grpcServer := server.New(cfg.Grpc, adminHandler, clientHandler, orderHandler)

	cmps := build.Components{
		pg,
		grpcServer,
	}

	app, err := build.NewApp(cmps)
	if err != nil {
		return err
	}

	return build.Run(ctx, app)
}
