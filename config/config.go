package config

import (
	"github.com/martketplace-vkr/pkg/server/grpc"
	"github.com/martketplace-vkr/pkg/build/components/pgxsqlxcomponent"
)

type Config struct {
	Grpc     grpc.Config             `validate:"required"`
	Postgres pgxsqlxcomponent.Config `validate:"required"`
}
