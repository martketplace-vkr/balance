package client

import (
	"context"

	clientpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Handler struct {
	service service
	clientpb.UnimplementedBalanceClientServiceServer
}

func New(service service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetWallet(ctx context.Context, req *clientpb.GetWalletRequest) (*clientpb.GetWalletResponse, error) {
	if err := ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}

	wallet, err := h.service.GetClientWallet(ctx, req.GetUserId())
	if err != nil {
		return nil, ToStatusError(err)
	}

	return &clientpb.GetWalletResponse{Wallet: wallet}, nil
}

func (h *Handler) GetDepositAddressList(ctx context.Context, req *clientpb.GetDepositAddressListRequest) (*clientpb.GetDepositAddressListResponse, error) {
	if err := ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}

	addresses, err := h.service.GetDepositAddressList(ctx, req.GetUserId())
	if err != nil {
		return nil, ToStatusError(err)
	}

	return &clientpb.GetDepositAddressListResponse{DepositAddresses: addresses}, nil
}

func (h *Handler) GetWalletTransactions(ctx context.Context, req *clientpb.GetWalletTransactionsRequest) (*clientpb.GetWalletTransactionsResponse, error) {
	if err := ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}

	var currencyCode *int64
	if req.CurrencyCode != nil {
		if req.GetCurrencyCode() <= 0 {
			return nil, status.Error(codes.InvalidArgument, "currency_code must be greater than zero")
		}
		value := req.GetCurrencyCode()
		currencyCode = &value
	}

	transactions, err := h.service.GetWalletTransactions(ctx, req.GetUserId(), currencyCode, req.GetLimit(), req.GetOffset())
	if err != nil {
		return nil, ToStatusError(err)
	}

	return &clientpb.GetWalletTransactionsResponse{Transactions: transactions}, nil
}

func (h *Handler) CreateTopUp(ctx context.Context, req *clientpb.CreateTopUpRequest) (*clientpb.CreateTopUpResponse, error) {
	if err := ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}

	topUp, err := h.service.CreateTopUp(ctx, req)
	if err != nil {
		return nil, ToStatusError(err)
	}

	return &clientpb.CreateTopUpResponse{TopUp: topUp}, nil
}

func (h *Handler) GetTopUp(ctx context.Context, req *clientpb.GetTopUpRequest) (*clientpb.GetTopUpResponse, error) {
	if err := ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}
	if err := ValidateID("top_up_id", req.GetTopUpId()); err != nil {
		return nil, err
	}

	topUp, err := h.service.GetTopUp(ctx, req.GetUserId(), req.GetTopUpId())
	if err != nil {
		return nil, ToStatusError(err)
	}

	return &clientpb.GetTopUpResponse{TopUp: topUp}, nil
}

func (h *Handler) GetTopUpList(ctx context.Context, req *clientpb.GetTopUpListRequest) (*clientpb.GetTopUpListResponse, error) {
	if err := ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}

	topUps, err := h.service.GetTopUpList(ctx, req.GetUserId(), req.GetLimit(), req.GetOffset())
	if err != nil {
		return nil, ToStatusError(err)
	}

	return &clientpb.GetTopUpListResponse{TopUps: topUps}, nil
}

func (h *Handler) CreateWithdrawal(ctx context.Context, req *clientpb.CreateWithdrawalRequest) (*clientpb.CreateWithdrawalResponse, error) {
	if err := ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}

	withdrawal, err := h.service.CreateWithdrawal(ctx, req)
	if err != nil {
		return nil, ToStatusError(err)
	}

	return &clientpb.CreateWithdrawalResponse{Withdrawal: withdrawal}, nil
}

func (h *Handler) GetWithdrawal(ctx context.Context, req *clientpb.GetWithdrawalRequest) (*clientpb.GetWithdrawalResponse, error) {
	if err := ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}
	if err := ValidateID("withdrawal_id", req.GetWithdrawalId()); err != nil {
		return nil, err
	}

	withdrawal, err := h.service.GetWithdrawal(ctx, req.GetUserId(), req.GetWithdrawalId())
	if err != nil {
		return nil, ToStatusError(err)
	}

	return &clientpb.GetWithdrawalResponse{Withdrawal: withdrawal}, nil
}
