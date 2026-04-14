package balance

import "errors"

var (
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrTopUpNotFound       = errors.New("top up not found")
	ErrWithdrawalNotFound  = errors.New("withdrawal not found")
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrInsufficientFunds   = errors.New("insufficient funds")
)
