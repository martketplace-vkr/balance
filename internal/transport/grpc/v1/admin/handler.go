package admin

import (
	"context"

	clienthandler "github.com/martketplace-vkr/balance/internal/transport/grpc/v1/client"
	adminpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/admin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Handler struct {
	service service
	adminpb.UnimplementedBalanceAdminServiceServer
}

func New(service service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetWallet(ctx context.Context, req *adminpb.GetWalletRequest) (*adminpb.GetWalletResponse, error) {
	if req.GetOwnerType() == 0 {
		return nil, status.Error(codes.InvalidArgument, "owner_type is required")
	}
	if err := clienthandler.ValidateID("owner_id", req.GetOwnerId()); err != nil {
		return nil, err
	}

	wallet, err := h.service.GetAdminWallet(ctx, req.GetOwnerType(), req.GetOwnerId())
	if err != nil {
		return nil, clienthandler.ToStatusError(err)
	}
	return &adminpb.GetWalletResponse{Wallet: wallet}, nil
}

func (h *Handler) GetTransaction(ctx context.Context, req *adminpb.GetTransactionRequest) (*adminpb.GetTransactionResponse, error) {
	if err := clienthandler.ValidateID("transaction_id", req.GetTransactionId()); err != nil {
		return nil, err
	}
	transaction, err := h.service.GetTransaction(ctx, req.GetTransactionId())
	if err != nil {
		return nil, clienthandler.ToStatusError(err)
	}
	return &adminpb.GetTransactionResponse{Transaction: transaction}, nil
}

func (h *Handler) PostAdjustment(ctx context.Context, req *adminpb.PostAdjustmentRequest) (*adminpb.PostAdjustmentResponse, error) {
	if req.GetOwnerType() == 0 {
		return nil, status.Error(codes.InvalidArgument, "owner_type is required")
	}
	if err := clienthandler.ValidateID("owner_id", req.GetOwnerId()); err != nil {
		return nil, err
	}

	transaction, err := h.service.PostAdjustment(ctx, req)
	if err != nil {
		return nil, clienthandler.ToStatusError(err)
	}
	return &adminpb.PostAdjustmentResponse{Transaction: transaction}, nil
}
