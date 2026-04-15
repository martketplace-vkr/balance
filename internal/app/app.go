package app

import (
	"context"

	// trmsqlx "github.com/avito-tech/go-transaction-manager/sqlx"
	// txmanager "github.com/avito-tech/go-transaction-manager/trm/manager"

	"github.com/martketplace-vkr/balance/config"
	inboxComponent "github.com/martketplace-vkr/balance/internal/app/cmp/inbox"
	outboxComponent "github.com/martketplace-vkr/balance/internal/app/cmp/outbox"
	"github.com/martketplace-vkr/balance/internal/app/cmp/server"
	repository "github.com/martketplace-vkr/balance/internal/repository/pg"
	servicebalance "github.com/martketplace-vkr/balance/internal/service/balance"
	adminTransport "github.com/martketplace-vkr/balance/internal/transport/grpc/v1/admin"
	clientTransport "github.com/martketplace-vkr/balance/internal/transport/grpc/v1/client"
	orderTransport "github.com/martketplace-vkr/balance/internal/transport/grpc/v1/order"
	"github.com/martketplace-vkr/balance/pkg/eventmapper"
	cryptowallet "github.com/martketplace-vkr/crypto-wallet/pkg/api/grpc/v1"

	"github.com/martketplace-vkr/pkg/build"
	"github.com/martketplace-vkr/pkg/build/components/pgxsqlxcomponent"
	"github.com/martketplace-vkr/pkg/kafkaconnector"
	outboxclient "github.com/martketplace-vkr/pkg/outbox"
)

func Run(ctx context.Context, cfg *config.Config) error {
	pg := pgxsqlxcomponent.New(cfg.Postgres)
	kafkaClient := kafkaconnector.NewClient(cfg.Kafka)
	kafkaProducer := kafkaClient.NewSyncProducer()

	outboxCl, err := outboxclient.NewDefaultWithOptions(
		cfg.Outbox.Outbox,
		outboxclient.WithSqlxDB(pg.DB),
		outboxclient.WithKafkaProducer(kafkaProducer),
	)
	if err != nil {
		return err
	}

	outboxCmp := outboxComponent.New(cfg.Outbox, outboxCl)
	cryptoWalletClient := cryptowallet.New(cfg.CryptoWallet)

	// txManager, err := txmanager.New(trmsqlx.NewDefaultFactory(pg.DB))
	// if err != nil {
	// 	return err
	// }

	repo := repository.New(pg.DB)
	service := servicebalance.New(repo, outboxCmp, cryptoWalletClient)
	adminHandler := adminTransport.New(service)
	clientHandler := clientTransport.New(service)
	orderHandler := orderTransport.New(service)
	inboxCmp := inboxComponent.New(
		cfg.Inbox,
		pg,
		kafkaClient,
		eventmapper.GetEventMapper(service),
	)

	grpcServer := server.New(cfg.Grpc, adminHandler, clientHandler, orderHandler)

	cmps := build.Components{
		pg,
		outboxCmp,
		cryptoWalletClient,
		inboxCmp,
		grpcServer,
	}

	app, err := build.NewApp(cmps)
	if err != nil {
		return err
	}

	return build.Run(ctx, app)
}
