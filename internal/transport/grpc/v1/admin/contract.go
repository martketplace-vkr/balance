package admin

import (
	"context"

	adminpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/admin"
	domainpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/domain"
)

type service interface {
	GetAdminWallet(ctx context.Context, ownerType domainpb.WalletOwnerType, ownerID int64) (*domainpb.Wallet, error)
	GetTransaction(ctx context.Context, transactionID int64) (*domainpb.LedgerTransaction, error)
	PostAdjustment(ctx context.Context, req *adminpb.PostAdjustmentRequest) (*domainpb.LedgerTransaction, error)
	ListTopUps(ctx context.Context, req *adminpb.ListTopUpsRequest) ([]*domainpb.TopUp, error)
	ConfirmTopUp(ctx context.Context, req *adminpb.ConfirmTopUpRequest) (*domainpb.TopUp, *domainpb.LedgerTransaction, error)
}
