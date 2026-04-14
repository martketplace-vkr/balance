package order

import (
	"context"

	domainpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/domain"
	orderpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/order"
)

type service interface {
	ReserveFunds(ctx context.Context, req *orderpb.ReserveFundsRequest) (*domainpb.LedgerTransaction, error)
	CaptureFunds(ctx context.Context, req *orderpb.CaptureFundsRequest) (*domainpb.LedgerTransaction, error)
	ReleaseFunds(ctx context.Context, req *orderpb.ReleaseFundsRequest) (*domainpb.LedgerTransaction, error)
	RefundFunds(ctx context.Context, req *orderpb.RefundFundsRequest) (*domainpb.LedgerTransaction, error)
}
