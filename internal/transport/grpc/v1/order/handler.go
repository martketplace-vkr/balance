package order

import (
	"context"

	clienthandler "github.com/martketplace-vkr/balance/internal/transport/grpc/v1/client"
	orderpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/order"
)

type Handler struct {
	service service
	orderpb.UnimplementedBalanceOrderServiceServer
}

func New(service service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ReserveFunds(ctx context.Context, req *orderpb.ReserveFundsRequest) (*orderpb.ReserveFundsResponse, error) {
	if err := clienthandler.ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}
	if err := clienthandler.ValidateID("order_id", req.GetOrderId()); err != nil {
		return nil, err
	}
	transaction, err := h.service.ReserveFunds(ctx, req)
	if err != nil {
		return nil, clienthandler.ToStatusError(err)
	}
	return &orderpb.ReserveFundsResponse{Transaction: transaction}, nil
}

func (h *Handler) CaptureFunds(ctx context.Context, req *orderpb.CaptureFundsRequest) (*orderpb.CaptureFundsResponse, error) {
	if err := clienthandler.ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}
	if err := clienthandler.ValidateID("order_id", req.GetOrderId()); err != nil {
		return nil, err
	}
	transaction, err := h.service.CaptureFunds(ctx, req)
	if err != nil {
		return nil, clienthandler.ToStatusError(err)
	}
	return &orderpb.CaptureFundsResponse{Transaction: transaction}, nil
}

func (h *Handler) ReleaseFunds(ctx context.Context, req *orderpb.ReleaseFundsRequest) (*orderpb.ReleaseFundsResponse, error) {
	if err := clienthandler.ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}
	if err := clienthandler.ValidateID("order_id", req.GetOrderId()); err != nil {
		return nil, err
	}
	transaction, err := h.service.ReleaseFunds(ctx, req)
	if err != nil {
		return nil, clienthandler.ToStatusError(err)
	}
	return &orderpb.ReleaseFundsResponse{Transaction: transaction}, nil
}

func (h *Handler) RefundFunds(ctx context.Context, req *orderpb.RefundFundsRequest) (*orderpb.RefundFundsResponse, error) {
	if err := clienthandler.ValidateID("user_id", req.GetUserId()); err != nil {
		return nil, err
	}
	if err := clienthandler.ValidateID("order_id", req.GetOrderId()); err != nil {
		return nil, err
	}
	transaction, err := h.service.RefundFunds(ctx, req)
	if err != nil {
		return nil, clienthandler.ToStatusError(err)
	}
	return &orderpb.RefundFundsResponse{Transaction: transaction}, nil
}
