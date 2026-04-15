package client

import (
	"context"

	clientpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/client"
	domainpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/domain"
)

type service interface {
	GetClientWallet(ctx context.Context, userID int64) (*domainpb.Wallet, error)
	GetDepositAddressList(ctx context.Context, userID int64) ([]*domainpb.DepositAddress, error)
	GetWalletTransactions(ctx context.Context, userID int64, currencyCode *int64, limit uint32, offset uint64) ([]*domainpb.LedgerTransaction, error)
	CreateTopUp(ctx context.Context, req *clientpb.CreateTopUpRequest) (*domainpb.TopUp, error)
	GetTopUp(ctx context.Context, userID, topUpID int64) (*domainpb.TopUp, error)
	GetTopUpList(ctx context.Context, userID int64, limit uint32, offset uint64) ([]*domainpb.TopUp, error)
	CreateWithdrawal(ctx context.Context, req *clientpb.CreateWithdrawalRequest) (*domainpb.Withdrawal, error)
	GetWithdrawal(ctx context.Context, userID, withdrawalID int64) (*domainpb.Withdrawal, error)
}
