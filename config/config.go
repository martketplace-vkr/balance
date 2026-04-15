package config

import (
	"github.com/martketplace-vkr/balance/internal/app/cmp/inbox"
	"github.com/martketplace-vkr/balance/internal/app/cmp/outbox"
	cryptowallet "github.com/martketplace-vkr/crypto-wallet/pkg/api/grpc/v1"
	"github.com/martketplace-vkr/pkg/build/components/pgxsqlxcomponent"
	"github.com/martketplace-vkr/pkg/kafkaconnector"
	"github.com/martketplace-vkr/pkg/server/grpc"
)

type Config struct {
	Grpc         grpc.Config                 `validate:"required"`
	Postgres     pgxsqlxcomponent.Config     `validate:"required"`
	Inbox        inbox.Config                `validate:"required"`
	Outbox       outbox.Config               `validate:"required"`
	CryptoWallet cryptowallet.Config         `validate:"required"`
	Kafka        kafkaconnector.ClientConfig `validate:"required"`
}
