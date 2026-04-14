package client

import (
	"errors"
	"fmt"

	servicebalance "github.com/martketplace-vkr/balance/internal/service/balance"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ValidateID(field string, value int64) error {
	if value <= 0 {
		return status.Errorf(codes.InvalidArgument, "%s must be greater than zero", field)
	}

	return nil
}

func ToStatusError(err error) error {
	switch {
	case errors.Is(err, servicebalance.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, servicebalance.ErrWalletNotFound),
		errors.Is(err, servicebalance.ErrTopUpNotFound),
		errors.Is(err, servicebalance.ErrWithdrawalNotFound),
		errors.Is(err, servicebalance.ErrTransactionNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, servicebalance.ErrInsufficientFunds):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return fmt.Errorf("balance transport: %w", err)
	}
}
