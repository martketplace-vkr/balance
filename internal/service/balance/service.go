package balance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	internaldomain "github.com/martketplace-vkr/balance/internal/domain"
	repository "github.com/martketplace-vkr/balance/internal/repository/pg"
	adminpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/admin"
	clientpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/client"
	domainpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/domain"
	orderpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/order"
	cryptowallet "github.com/martketplace-vkr/crypto-wallet/pkg/api/grpc/v1"
	cryptowalletpb "github.com/martketplace-vkr/crypto-wallet/pkg/api/grpc/v1/client"
	"github.com/martketplace-vkr/pkg/utils/currency"
)

type Service struct {
	repository   *repository.Repository
	outbox       outbox
	cryptoWallet *cryptowallet.Connector
}

type outbox interface {
	Send(ctx context.Context, eventType string, payload any) error
}

func New(repository *repository.Repository, outbox outbox, cryptoWallet *cryptowallet.Connector) *Service {
	return &Service{
		repository:   repository,
		outbox:       outbox,
		cryptoWallet: cryptoWallet,
	}
}

func (s *Service) GetClientWallet(ctx context.Context, userID int64) (*domainpb.Wallet, error) {
	return s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, userID)
}

func (s *Service) GetWalletTransactions(ctx context.Context, userID int64, currencyCode *int64, limit uint32, offset uint64) ([]*domainpb.LedgerTransaction, error) {
	wallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, userID)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 50
	}

	return s.repository.ListWalletTransactions(ctx, wallet.Id, currencyCode, limit, offset)
}

func (s *Service) CreateTopUp(ctx context.Context, req *clientpb.CreateTopUpRequest) (*domainpb.TopUp, error) {
	if req.GetMoney() == nil {
		return nil, fmt.Errorf("%w: money is required", ErrInvalidArgument)
	}
	if _, err := parsePositiveAmount(req.GetMoney().GetAmount()); err != nil {
		return nil, err
	}
	if req.GetMoney().GetCurrencyCode() <= 0 {
		return nil, fmt.Errorf("%w: currency_code must be greater than zero", ErrInvalidArgument)
	}
	if req.GetProviderType() == domainpb.ProviderType_PROVIDER_TYPE_UNSPECIFIED {
		return nil, fmt.Errorf("%w: provider_type is required", ErrInvalidArgument)
	}
	if req.GetProviderName() == "" || req.GetIdempotencyKey() == "" {
		return nil, fmt.Errorf("%w: provider_name and idempotency_key are required", ErrInvalidArgument)
	}

	wallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, req.GetUserId())
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	createReq := repository.TopUpCreateRequest{
		WalletID:       wallet.Id,
		CurrencyCode:   req.GetMoney().GetCurrencyCode(),
		Amount:         req.GetMoney().GetAmount(),
		ProviderType:   req.GetProviderType(),
		ProviderName:   req.GetProviderName(),
		Status:         domainpb.TopUpStatus_TOP_UP_STATUS_CREATED,
		ExternalID:     fmt.Sprintf("topup-%d", time.Now().UnixNano()),
		ExternalStatus: "created",
		IdempotencyKey: req.GetIdempotencyKey(),
		ExpiresAt:      &expiresAt,
	}

	switch req.GetProviderType() {
	case domainpb.ProviderType_PROVIDER_TYPE_ACQUIRING:
		createReq.PaymentURL = fmt.Sprintf("https://payments.local/top-ups/%s", req.GetIdempotencyKey())
	case domainpb.ProviderType_PROVIDER_TYPE_CRYPTO:
		if req.GetMoney().GetCurrencyCode() != int64(currency.USDTinTRC) {
			return nil, fmt.Errorf("%w: only USDT-TRC20 is supported", ErrInvalidArgument)
		}
		if req.GetNetwork() != "TRON" {
			return nil, fmt.Errorf("%w: only TRON network is supported", ErrInvalidArgument)
		}
		if s.cryptoWallet == nil || s.cryptoWallet.Client == nil {
			return nil, fmt.Errorf("%w: crypto wallet client is not configured", ErrInvalidArgument)
		}

		addressResp, err := s.cryptoWallet.Client.GetOrCreateDepositAddress(ctx, &cryptowalletpb.GetOrCreateDepositAddressRequest{
			UserId:  req.GetUserId(),
			Network: req.GetNetwork(),
			Asset:   "USDT",
		})
		if err != nil {
			return nil, err
		}
		if addressResp.GetDepositAddress() == nil || addressResp.GetDepositAddress().GetAddress() == "" {
			return nil, fmt.Errorf("%w: empty deposit address received", ErrInvalidArgument)
		}

		createReq.WalletAddress = addressResp.GetDepositAddress().GetAddress()
		createReq.Network = req.GetNetwork()
	default:
		return nil, fmt.Errorf("%w: unsupported provider_type", ErrInvalidArgument)
	}

	topUp, err := s.repository.CreateTopUp(ctx, createReq)
	if err != nil {
		return nil, err
	}

	if req.GetProviderType() == domainpb.ProviderType_PROVIDER_TYPE_CRYPTO && s.outbox != nil {
		if err := s.outbox.Send(ctx, "blockchain_watch_address_register", internaldomain.RegisterWatchAddressEvent{
			UserID:   req.GetUserId(),
			Address:  topUp.GetWalletAddress(),
			Network:  topUp.GetNetwork(),
			Asset:    "USDT",
			Source:   "balance_topup",
			Provider: req.GetProviderName(),
		}); err != nil {
			return nil, err
		}
	}

	return topUp, nil
}

func (s *Service) GetTopUp(ctx context.Context, userID, topUpID int64) (*domainpb.TopUp, error) {
	topUp, err := s.repository.GetTopUpByUser(ctx, userID, topUpID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTopUpNotFound
		}
		return nil, err
	}
	return topUp, nil
}

func (s *Service) GetTopUpList(ctx context.Context, userID int64, limit uint32, offset uint64) ([]*domainpb.TopUp, error) {
	if limit == 0 {
		limit = 50
	}
	return s.repository.ListTopUpsByUser(ctx, userID, limit, offset)
}

func (s *Service) CreateWithdrawal(ctx context.Context, req *clientpb.CreateWithdrawalRequest) (*domainpb.Withdrawal, error) {
	if req.GetMoney() == nil {
		return nil, fmt.Errorf("%w: money is required", ErrInvalidArgument)
	}
	amount, err := parsePositiveAmount(req.GetMoney().GetAmount())
	if err != nil {
		return nil, err
	}
	if req.GetDestinationType() == domainpb.DestinationType_DESTINATION_TYPE_UNSPECIFIED || req.GetDestination() == "" || req.GetIdempotencyKey() == "" {
		return nil, fmt.Errorf("%w: destination_type, destination and idempotency_key are required", ErrInvalidArgument)
	}

	userWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, req.GetUserId())
	if err != nil {
		return nil, err
	}
	systemWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_SYSTEM, 0)
	if err != nil {
		return nil, err
	}

	userAvailable, err := s.repository.GetOrCreateAccount(ctx, userWallet.Id, req.GetMoney().GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, err
	}
	systemAvailable, err := s.repository.GetOrCreateAccount(ctx, systemWallet.Id, req.GetMoney().GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, err
	}

	userBalance, err := s.repository.GetAccountBalance(ctx, userAvailable.Id)
	if err != nil {
		return nil, err
	}
	if userBalance.Cmp(amount) < 0 {
		return nil, ErrInsufficientFunds
	}

	if _, err = s.postIfNotExists(ctx, repository.LedgerWriteRequest{
		IdempotencyKey:  req.GetIdempotencyKey(),
		Type:            domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_WITHDRAWAL,
		Status:          domainpb.LedgerTransactionStatus_LEDGER_TRANSACTION_STATUS_POSTED,
		Reason:          "withdrawal request",
		ReferenceType:   domainpb.ReferenceType_REFERENCE_TYPE_WITHDRAWAL,
		ReferenceID:     req.GetIdempotencyKey(),
		DebitAccountID:  userAvailable.Id,
		CreditAccountID: systemAvailable.Id,
		Amount:          formatAmount(amount),
		PostedAt:        time.Now(),
	}); err != nil {
		return nil, err
	}

	return s.repository.CreateWithdrawal(ctx, repository.WithdrawalCreateRequest{
		WalletID:        userWallet.Id,
		CurrencyCode:    req.GetMoney().GetCurrencyCode(),
		Amount:          formatAmount(amount),
		DestinationType: req.GetDestinationType(),
		Destination:     req.GetDestination(),
		Status:          domainpb.WithdrawalStatus_WITHDRAWAL_STATUS_PENDING,
		ExternalID:      fmt.Sprintf("wd-%d", time.Now().UnixNano()),
		IdempotencyKey:  req.GetIdempotencyKey(),
	})
}

func (s *Service) GetWithdrawal(ctx context.Context, userID, withdrawalID int64) (*domainpb.Withdrawal, error) {
	withdrawal, err := s.repository.GetWithdrawalByUser(ctx, userID, withdrawalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWithdrawalNotFound
		}
		return nil, err
	}
	return withdrawal, nil
}

func (s *Service) ReserveFunds(ctx context.Context, req *orderpb.ReserveFundsRequest) (*domainpb.LedgerTransaction, error) {
	return s.moveBetweenUserAccounts(ctx, req.GetUserId(), req.GetOrderId(), req.GetMoney(), req.GetIdempotencyKey(), req.GetReason(), domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_HOLD, domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE, domainpb.AccountType_ACCOUNT_TYPE_HOLD)
}

func (s *Service) CaptureFunds(ctx context.Context, req *orderpb.CaptureFundsRequest) (*domainpb.LedgerTransaction, error) {
	return s.moveUserToSystem(ctx, req.GetUserId(), req.GetOrderId(), req.GetMoney(), req.GetIdempotencyKey(), req.GetReason(), domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_CAPTURE, domainpb.AccountType_ACCOUNT_TYPE_HOLD)
}

func (s *Service) ReleaseFunds(ctx context.Context, req *orderpb.ReleaseFundsRequest) (*domainpb.LedgerTransaction, error) {
	return s.moveBetweenUserAccounts(ctx, req.GetUserId(), req.GetOrderId(), req.GetMoney(), req.GetIdempotencyKey(), req.GetReason(), domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_RELEASE, domainpb.AccountType_ACCOUNT_TYPE_HOLD, domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
}

func (s *Service) RefundFunds(ctx context.Context, req *orderpb.RefundFundsRequest) (*domainpb.LedgerTransaction, error) {
	return s.moveSystemToUser(ctx, req.GetUserId(), req.GetOrderId(), req.GetMoney(), req.GetIdempotencyKey(), req.GetReason(), domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_REFUND, domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
}

func (s *Service) GetAdminWallet(ctx context.Context, ownerType domainpb.WalletOwnerType, ownerID int64) (*domainpb.Wallet, error) {
	wallet, err := s.repository.GetWalletByOwner(ctx, ownerType, ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, err
	}
	return wallet, nil
}

func (s *Service) GetTransaction(ctx context.Context, transactionID int64) (*domainpb.LedgerTransaction, error) {
	transaction, err := s.repository.GetTransaction(ctx, transactionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, err
	}
	return transaction, nil
}

func (s *Service) PostAdjustment(ctx context.Context, req *adminpb.PostAdjustmentRequest) (*domainpb.LedgerTransaction, error) {
	if req.GetMoney() == nil || req.GetIdempotencyKey() == "" {
		return nil, fmt.Errorf("%w: money and idempotency_key are required", ErrInvalidArgument)
	}

	amount, err := parseAmount(req.GetMoney().GetAmount())
	if err != nil {
		return nil, err
	}
	if amount.Sign() == 0 {
		return nil, fmt.Errorf("%w: amount must not be zero", ErrInvalidArgument)
	}

	targetWallet, err := s.repository.EnsureWallet(ctx, req.GetOwnerType(), req.GetOwnerId())
	if err != nil {
		return nil, err
	}
	systemWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_SYSTEM, 0)
	if err != nil {
		return nil, err
	}

	targetAccount, err := s.repository.GetOrCreateAccount(ctx, targetWallet.Id, req.GetMoney().GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, err
	}
	systemAccount, err := s.repository.GetOrCreateAccount(ctx, systemWallet.Id, req.GetMoney().GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, err
	}

	writeReq := repository.LedgerWriteRequest{
		IdempotencyKey: req.GetIdempotencyKey(),
		Type:           domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_ADJUSTMENT,
		Status:         domainpb.LedgerTransactionStatus_LEDGER_TRANSACTION_STATUS_POSTED,
		Reason:         req.GetReason(),
		ReferenceType:  domainpb.ReferenceType_REFERENCE_TYPE_MANUAL_OPERATION,
		ReferenceID:    req.GetIdempotencyKey(),
		Amount:         formatAmount(absRat(amount)),
		PostedAt:       time.Now(),
	}

	if amount.Sign() > 0 {
		writeReq.DebitAccountID = systemAccount.Id
		writeReq.CreditAccountID = targetAccount.Id
	} else {
		targetBalance, err := s.repository.GetAccountBalance(ctx, targetAccount.Id)
		if err != nil {
			return nil, err
		}
		if targetBalance.Cmp(absRat(amount)) < 0 {
			return nil, ErrInsufficientFunds
		}
		writeReq.DebitAccountID = targetAccount.Id
		writeReq.CreditAccountID = systemAccount.Id
	}

	return s.postIfNotExists(ctx, writeReq)
}

func (s *Service) moveBetweenUserAccounts(ctx context.Context, userID, orderID int64, money *domainpb.Money, idempotencyKey, reason string, txType domainpb.LedgerTransactionType, fromType, toType domainpb.AccountType) (*domainpb.LedgerTransaction, error) {
	if money == nil {
		return nil, fmt.Errorf("%w: money is required", ErrInvalidArgument)
	}
	amount, err := parsePositiveAmount(money.GetAmount())
	if err != nil {
		return nil, err
	}

	wallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, userID)
	if err != nil {
		return nil, err
	}
	fromAccount, err := s.repository.GetOrCreateAccount(ctx, wallet.Id, money.GetCurrencyCode(), fromType)
	if err != nil {
		return nil, err
	}
	toAccount, err := s.repository.GetOrCreateAccount(ctx, wallet.Id, money.GetCurrencyCode(), toType)
	if err != nil {
		return nil, err
	}
	balance, err := s.repository.GetAccountBalance(ctx, fromAccount.Id)
	if err != nil {
		return nil, err
	}
	if balance.Cmp(amount) < 0 {
		return nil, ErrInsufficientFunds
	}

	return s.postIfNotExists(ctx, repository.LedgerWriteRequest{
		IdempotencyKey:  idempotencyKey,
		Type:            txType,
		Status:          domainpb.LedgerTransactionStatus_LEDGER_TRANSACTION_STATUS_POSTED,
		Reason:          reason,
		ReferenceType:   domainpb.ReferenceType_REFERENCE_TYPE_ORDER,
		ReferenceID:     fmt.Sprintf("%d", orderID),
		DebitAccountID:  fromAccount.Id,
		CreditAccountID: toAccount.Id,
		Amount:          formatAmount(amount),
		PostedAt:        time.Now(),
	})
}

func (s *Service) moveUserToSystem(ctx context.Context, userID, orderID int64, money *domainpb.Money, idempotencyKey, reason string, txType domainpb.LedgerTransactionType, fromType domainpb.AccountType) (*domainpb.LedgerTransaction, error) {
	if money == nil {
		return nil, fmt.Errorf("%w: money is required", ErrInvalidArgument)
	}
	amount, err := parsePositiveAmount(money.GetAmount())
	if err != nil {
		return nil, err
	}

	userWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, userID)
	if err != nil {
		return nil, err
	}
	systemWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_SYSTEM, 0)
	if err != nil {
		return nil, err
	}
	userAccount, err := s.repository.GetOrCreateAccount(ctx, userWallet.Id, money.GetCurrencyCode(), fromType)
	if err != nil {
		return nil, err
	}
	systemAccount, err := s.repository.GetOrCreateAccount(ctx, systemWallet.Id, money.GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, err
	}
	balance, err := s.repository.GetAccountBalance(ctx, userAccount.Id)
	if err != nil {
		return nil, err
	}
	if balance.Cmp(amount) < 0 {
		return nil, ErrInsufficientFunds
	}

	return s.postIfNotExists(ctx, repository.LedgerWriteRequest{
		IdempotencyKey:  idempotencyKey,
		Type:            txType,
		Status:          domainpb.LedgerTransactionStatus_LEDGER_TRANSACTION_STATUS_POSTED,
		Reason:          reason,
		ReferenceType:   domainpb.ReferenceType_REFERENCE_TYPE_ORDER,
		ReferenceID:     fmt.Sprintf("%d", orderID),
		DebitAccountID:  userAccount.Id,
		CreditAccountID: systemAccount.Id,
		Amount:          formatAmount(amount),
		PostedAt:        time.Now(),
	})
}

func (s *Service) moveSystemToUser(ctx context.Context, userID, orderID int64, money *domainpb.Money, idempotencyKey, reason string, txType domainpb.LedgerTransactionType, toType domainpb.AccountType) (*domainpb.LedgerTransaction, error) {
	if money == nil {
		return nil, fmt.Errorf("%w: money is required", ErrInvalidArgument)
	}
	amount, err := parsePositiveAmount(money.GetAmount())
	if err != nil {
		return nil, err
	}

	userWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, userID)
	if err != nil {
		return nil, err
	}
	systemWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_SYSTEM, 0)
	if err != nil {
		return nil, err
	}
	userAccount, err := s.repository.GetOrCreateAccount(ctx, userWallet.Id, money.GetCurrencyCode(), toType)
	if err != nil {
		return nil, err
	}
	systemAccount, err := s.repository.GetOrCreateAccount(ctx, systemWallet.Id, money.GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, err
	}
	systemBalance, err := s.repository.GetAccountBalance(ctx, systemAccount.Id)
	if err != nil {
		return nil, err
	}
	if systemBalance.Cmp(amount) < 0 {
		return nil, ErrInsufficientFunds
	}

	return s.postIfNotExists(ctx, repository.LedgerWriteRequest{
		IdempotencyKey:  idempotencyKey,
		Type:            txType,
		Status:          domainpb.LedgerTransactionStatus_LEDGER_TRANSACTION_STATUS_POSTED,
		Reason:          reason,
		ReferenceType:   domainpb.ReferenceType_REFERENCE_TYPE_ORDER,
		ReferenceID:     fmt.Sprintf("%d", orderID),
		DebitAccountID:  systemAccount.Id,
		CreditAccountID: userAccount.Id,
		Amount:          formatAmount(amount),
		PostedAt:        time.Now(),
	})
}

func (s *Service) postIfNotExists(ctx context.Context, req repository.LedgerWriteRequest) (*domainpb.LedgerTransaction, error) {
	transaction, err := s.repository.GetTransactionByIdempotencyKey(ctx, req.IdempotencyKey)
	if err == nil {
		return transaction, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.repository.PostLedgerTransaction(ctx, req)
}
