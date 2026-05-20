package balance

import (
	"fmt"
	"math/big"
	"strings"
)

func parseAmount(raw string) (*big.Rat, error) {
	value, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("%w: invalid amount", ErrInvalidArgument)
	}

	return value, nil
}

func parsePositiveAmount(raw string) (*big.Rat, error) {
	value, err := parseAmount(raw)
	if err != nil {
		return nil, err
	}
	if value.Sign() <= 0 {
		return nil, fmt.Errorf("%w: amount must be greater than zero", ErrInvalidArgument)
	}

	return value, nil
}

func formatAmount(value *big.Rat) string {
	if value == nil {
		return "0.00000000"
	}

	return value.FloatString(8)
}

func absRat(value *big.Rat) *big.Rat {
	if value.Sign() >= 0 {
		return value
	}

	return new(big.Rat).Neg(value)
}

func rubProviderMethod(providerName string) (string, error) {
	switch strings.TrimSpace(providerName) {
	case mockRubSBPProvider:
		return "sbp", nil
	case mockRubCardProvider:
		return "card", nil
	default:
		return "", fmt.Errorf("%w: unsupported RUB acquiring provider", ErrInvalidArgument)
	}
}
