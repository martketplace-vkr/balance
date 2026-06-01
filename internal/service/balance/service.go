package balance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	internaldomain "github.com/martketplace-vkr/balance/internal/domain"
	repository "github.com/martketplace-vkr/balance/internal/repository/pg"
	adminpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/admin"
	clientpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/client"
	domainpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/domain"
	orderpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/order"
	cryptowallet "github.com/martketplace-vkr/crypto-wallet/pkg/api/grpc/v1"
	cryptowalletpb "github.com/martketplace-vkr/crypto-wallet/pkg/api/grpc/v1/client"
	"github.com/martketplace-vkr/pkg/inbox/dto"
	"github.com/martketplace-vkr/pkg/utils/currency"
)

type userSignUpEvent struct {
	ID int64 `json:"ID"`
}

type cryptoDepositConfirmedEvent struct {
	UserID        int64  `json:"user_id"`
	Address       string `json:"address"`
	Network       string `json:"network"`
	Asset         string `json:"asset"`
	TxHash        string `json:"tx_hash"`
	LogIndex      int64  `json:"log_index"`
	Amount        string `json:"amount"`
	BlockNumber   int64  `json:"block_number"`
	Confirmations int64  `json:"confirmations"`
}

const (
	cryptoNetwork       = "TRON"
	cryptoAsset         = "USDT"
	cryptoProviderName  = "USDT-TRC20"
	mockRubSBPProvider  = "MOCK_RUB_SBP"
	mockRubCardProvider = "MOCK_RUB_CARD"
	defaultProviderURL  = "http://127.0.0.1:8010"
)

type Service struct {
	repository            *repository.Repository
	outbox                outbox
	cryptoWallet          *cryptowallet.Connector
	mockProviderPublicURL string
}

type outbox interface {
	Send(ctx context.Context, eventType string, payload any) error
}

func New(repository *repository.Repository, outbox outbox, cryptoWallet *cryptowallet.Connector, mockProviderPublicURL string) *Service {
	if strings.TrimSpace(mockProviderPublicURL) == "" {
		mockProviderPublicURL = defaultProviderURL
	}

	return &Service{
		repository:            repository,
		outbox:                outbox,
		cryptoWallet:          cryptoWallet,
		mockProviderPublicURL: strings.TrimRight(strings.TrimSpace(mockProviderPublicURL), "/"),
	}
}

func (s *Service) GetClientWallet(ctx context.Context, userID int64) (*domainpb.Wallet, error) {
	return s.ensureUserAccounts(ctx, userID)
}

func (s *Service) HandleUserSignUp(ctx context.Context, event dto.Event) error {
	var payload userSignUpEvent
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if payload.ID <= 0 {
		return fmt.Errorf("%w: user_id must be greater than zero", ErrInvalidArgument)
	}

	return s.EnsureUserWalletAccounts(ctx, payload.ID)
}

func (s *Service) HandleCryptoDepositConfirmed(ctx context.Context, event dto.Event) error {
	var payload cryptoDepositConfirmedEvent
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if payload.UserID <= 0 {
		return fmt.Errorf("%w: user_id must be greater than zero", ErrInvalidArgument)
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Network), cryptoNetwork) {
		return fmt.Errorf("%w: only TRON network is supported", ErrInvalidArgument)
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Asset), cryptoAsset) {
		return fmt.Errorf("%w: only USDT asset is supported", ErrInvalidArgument)
	}
	if strings.TrimSpace(payload.TxHash) == "" {
		return fmt.Errorf("%w: tx_hash is required", ErrInvalidArgument)
	}
	amount, err := parsePositiveAmount(payload.Amount)
	if err != nil {
		return err
	}

	txHash := strings.TrimSpace(payload.TxHash)
	externalID := fmt.Sprintf("%s:%d", txHash, payload.LogIndex)
	ledgerKey := fmt.Sprintf("crypto-deposit:%s:%s:%d", strings.ToUpper(strings.TrimSpace(payload.Network)), txHash, payload.LogIndex)

	existingTopUp, err := s.repository.GetTopUpByProviderTxHash(ctx, domainpb.ProviderType_PROVIDER_TYPE_CRYPTO, txHash)
	wasConfirmed := err == nil && existingTopUp.GetStatus() == domainpb.TopUpStatus_TOP_UP_STATUS_CONFIRMED
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	userWallet, err := s.ensureUserAccounts(ctx, payload.UserID)
	if err != nil {
		return err
	}
	systemWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_SYSTEM, 0)
	if err != nil {
		return err
	}
	userAccount, err := s.repository.GetOrCreateAccount(ctx, userWallet.GetId(), int64(currency.USDTinTRC), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return err
	}
	systemAccount, err := s.repository.GetOrCreateAccount(ctx, systemWallet.GetId(), int64(currency.USDTinTRC), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return err
	}

	now := time.Now()
	topUp := existingTopUp
	if topUp == nil {
		topUp, err = s.repository.CreateTopUp(ctx, repository.TopUpCreateRequest{
			WalletID:       userWallet.GetId(),
			CurrencyCode:   int64(currency.USDTinTRC),
			Amount:         formatAmount(amount),
			ProviderType:   domainpb.ProviderType_PROVIDER_TYPE_CRYPTO,
			ProviderName:   cryptoProviderName,
			Status:         domainpb.TopUpStatus_TOP_UP_STATUS_CONFIRMED,
			ExternalID:     externalID,
			ExternalStatus: "confirmed",
			WalletAddress:  strings.TrimSpace(payload.Address),
			Network:        cryptoNetwork,
			TxHash:         txHash,
			IdempotencyKey: ledgerKey,
			PaidAt:         &now,
			ConfirmedAt:    &now,
		})
		if err != nil {
			return err
		}
	}

	_, err = s.postIfNotExists(ctx, repository.LedgerWriteRequest{
		IdempotencyKey:  ledgerKey,
		Type:            domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TOP_UP,
		Status:          domainpb.LedgerTransactionStatus_LEDGER_TRANSACTION_STATUS_POSTED,
		Reason:          "confirmed crypto deposit",
		ReferenceType:   domainpb.ReferenceType_REFERENCE_TYPE_TOP_UP,
		ReferenceID:     topUp.GetExternalId(),
		DebitAccountID:  systemAccount.GetId(),
		CreditAccountID: userAccount.GetId(),
		Amount:          formatAmount(amount),
		PostedAt:        now,
	})
	if err != nil {
		return err
	}

	if s.outbox != nil && !wasConfirmed {
		return s.outbox.Send(ctx, "balance_topup_completed", struct {
			UserID       int64  `json:"user_id"`
			CurrencyCode int64  `json:"currency_code"`
			Amount       string `json:"amount"`
			TopUpID      int64  `json:"top_up_id"`
			TxHash       string `json:"tx_hash"`
		}{
			UserID:       payload.UserID,
			CurrencyCode: int64(currency.USDTinTRC),
			Amount:       formatAmount(amount),
			TopUpID:      topUp.GetId(),
			TxHash:       txHash,
		})
	}

	return nil
}

func (s *Service) EnsureUserWalletAccounts(ctx context.Context, userID int64) error {
	if _, err := s.ensureUserAccounts(ctx, userID); err != nil {
		return err
	}

	_, err := s.ensureUserCryptoDepositAddress(ctx, userID)
	return err
}

func (s *Service) ensureUserAccounts(ctx context.Context, userID int64) (*domainpb.Wallet, error) {
	wallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, userID)
	if err != nil {
		return nil, err
	}

	for _, currencyCode := range []int64{int64(currency.RUB), int64(currency.USDTinTRC)} {
		if _, err := s.repository.GetOrCreateAccount(ctx, wallet.Id, currencyCode, domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE); err != nil {
			return nil, err
		}
		if _, err := s.repository.GetOrCreateAccount(ctx, wallet.Id, currencyCode, domainpb.AccountType_ACCOUNT_TYPE_HOLD); err != nil {
			return nil, err
		}
	}

	return s.repository.GetWalletByID(ctx, wallet.Id)
}

func (s *Service) GetDepositAddressList(ctx context.Context, userID int64) ([]*domainpb.DepositAddress, error) {
	if _, err := s.ensureUserAccounts(ctx, userID); err != nil {
		return nil, err
	}
	if _, err := s.ensureUserCryptoDepositAddress(ctx, userID); err != nil {
		return nil, err
	}

	return s.repository.ListDepositAddressesByUser(ctx, userID)
}

func (s *Service) ensureUserCryptoDepositAddress(ctx context.Context, userID int64) (*domainpb.DepositAddress, error) {
	if s.cryptoWallet == nil || s.cryptoWallet.Client == nil {
		return nil, fmt.Errorf("%w: crypto wallet client is not configured", ErrInvalidArgument)
	}

	addressResp, err := s.cryptoWallet.Client.GetOrCreateDepositAddress(ctx, &cryptowalletpb.GetOrCreateDepositAddressRequest{
		UserId:  userID,
		Network: cryptoNetwork,
		Asset:   cryptoAsset,
	})
	if err != nil {
		return nil, err
	}
	if addressResp.GetDepositAddress() == nil || addressResp.GetDepositAddress().GetAddress() == "" {
		return nil, fmt.Errorf("%w: empty deposit address received", ErrInvalidArgument)
	}

	address, created, err := s.repository.SaveDepositAddress(ctx, userID, addressResp.GetDepositAddress().GetAddress(), cryptoNetwork)
	if err != nil {
		return nil, err
	}

	if created && s.outbox != nil {
		if err := s.outbox.Send(ctx, "blockchain_watch_address_register", internaldomain.RegisterWatchAddressEvent{
			UserID:   userID,
			Address:  address.GetAddress(),
			Network:  address.GetNetwork(),
			Asset:    cryptoAsset,
			Source:   "balance_wallet_provisioning",
			Provider: "crypto-wallet",
		}); err != nil {
			return nil, err
		}
	}

	return address, nil
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

	wallet, err := s.ensureUserAccounts(ctx, req.GetUserId())
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
		if req.GetMoney().GetCurrencyCode() != int64(currency.RUB) {
			return nil, fmt.Errorf("%w: only RUB is supported for acquiring top ups", ErrInvalidArgument)
		}
		method, err := rubProviderMethod(req.GetProviderName())
		if err != nil {
			return nil, err
		}
		createReq.Status = domainpb.TopUpStatus_TOP_UP_STATUS_PENDING
		createReq.PaymentURL = fmt.Sprintf("%s/pay/%s?amount=%s&method=%s",
			s.mockProviderPublicURL,
			url.PathEscape(createReq.ExternalID),
			url.QueryEscape(req.GetMoney().GetAmount()),
			url.QueryEscape(method),
		)
	case domainpb.ProviderType_PROVIDER_TYPE_CRYPTO:
		if req.GetMoney().GetCurrencyCode() != int64(currency.USDTinTRC) {
			return nil, fmt.Errorf("%w: only USDT-TRC20 is supported", ErrInvalidArgument)
		}
		if req.GetNetwork() != "TRON" {
			return nil, fmt.Errorf("%w: only TRON network is supported", ErrInvalidArgument)
		}
		address, err := s.ensureUserCryptoDepositAddress(ctx, req.GetUserId())
		if err != nil {
			return nil, err
		}

		createReq.WalletAddress = address.GetAddress()
		createReq.Network = address.GetNetwork()
	default:
		return nil, fmt.Errorf("%w: unsupported provider_type", ErrInvalidArgument)
	}

	topUp, err := s.repository.CreateTopUp(ctx, createReq)
	if err != nil {
		return nil, err
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

func (s *Service) ListTopUps(ctx context.Context, req *adminpb.ListTopUpsRequest) ([]*domainpb.TopUp, error) {
	limit := req.GetLimit()
	if limit == 0 {
		limit = 50
	}

	filter := repository.TopUpListFilter{
		Limit:  limit,
		Offset: req.GetOffset(),
	}
	if req.CurrencyCode != nil {
		currencyCode := req.GetCurrencyCode()
		filter.CurrencyCode = &currencyCode
	}
	if req.ProviderType != nil {
		providerType := req.GetProviderType()
		filter.ProviderType = &providerType
	}
	if req.Status != nil {
		status := req.GetStatus()
		filter.Status = &status
	}

	return s.repository.ListTopUps(ctx, filter)
}

func (s *Service) ConfirmTopUp(ctx context.Context, req *adminpb.ConfirmTopUpRequest) (*domainpb.TopUp, *domainpb.LedgerTransaction, error) {
	externalID := strings.TrimSpace(req.GetExternalId())
	providerName := strings.TrimSpace(req.GetProviderName())
	webhookEventID := strings.TrimSpace(req.GetWebhookEventId())
	if externalID == "" || providerName == "" || webhookEventID == "" {
		return nil, nil, fmt.Errorf("%w: external_id, provider_name and webhook_event_id are required", ErrInvalidArgument)
	}
	if req.GetMoney() == nil {
		return nil, nil, fmt.Errorf("%w: money is required", ErrInvalidArgument)
	}
	amount, err := parsePositiveAmount(req.GetMoney().GetAmount())
	if err != nil {
		return nil, nil, err
	}

	topUp, err := s.repository.GetTopUpByProviderExternalID(ctx, domainpb.ProviderType_PROVIDER_TYPE_ACQUIRING, providerName, externalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrTopUpNotFound
		}
		return nil, nil, err
	}
	if topUp.GetMoney().GetCurrencyCode() != int64(currency.RUB) || req.GetMoney().GetCurrencyCode() != int64(currency.RUB) {
		return nil, nil, fmt.Errorf("%w: only RUB confirmation is supported", ErrInvalidArgument)
	}
	topUpAmount, err := parsePositiveAmount(topUp.GetMoney().GetAmount())
	if err != nil {
		return nil, nil, err
	}
	if topUpAmount.Cmp(amount) != 0 {
		return nil, nil, fmt.Errorf("%w: top up amount mismatch", ErrInvalidArgument)
	}
	wasConfirmed := topUp.GetStatus() == domainpb.TopUpStatus_TOP_UP_STATUS_CONFIRMED
	if wasConfirmed {
		return topUp, nil, nil
	}
	if topUp.GetStatus() != domainpb.TopUpStatus_TOP_UP_STATUS_PENDING && topUp.GetStatus() != domainpb.TopUpStatus_TOP_UP_STATUS_CREATED {
		return nil, nil, fmt.Errorf("%w: top up is not confirmable", ErrInvalidArgument)
	}

	userWallet, err := s.repository.GetWalletByID(ctx, topUp.GetWalletId())
	if err != nil {
		return nil, nil, err
	}
	systemWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_SYSTEM, 0)
	if err != nil {
		return nil, nil, err
	}
	userAccount, err := s.repository.GetOrCreateAccount(ctx, userWallet.GetId(), int64(currency.RUB), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, nil, err
	}
	systemAccount, err := s.repository.GetOrCreateAccount(ctx, systemWallet.GetId(), int64(currency.RUB), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, nil, err
	}

	ledgerKey := "mock-provider-topup-" + webhookEventID
	confirmedTopUp, transaction, err := s.repository.ConfirmTopUp(ctx, repository.TopUpConfirmRequest{
		TopUpID:         topUp.GetId(),
		IdempotencyKey:  ledgerKey,
		Reason:          "confirmed mock provider RUB top up",
		DebitAccountID:  systemAccount.GetId(),
		CreditAccountID: userAccount.GetId(),
		Amount:          formatAmount(amount),
		ExternalStatus:  "confirmed",
	})
	if err != nil {
		return nil, nil, err
	}

	if s.outbox != nil && !wasConfirmed {
		if err := s.outbox.Send(ctx, "balance_topup_completed", struct {
			UserID int64 `json:"user_id"`
		}{UserID: userWallet.GetOwnerId()}); err != nil {
			return nil, nil, err
		}
	}

	return confirmedTopUp, transaction, nil
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
	if req.GetVendorId() > 0 || req.GetVendorMoney() != nil || req.GetMarketplaceFee() != nil {
		return s.captureSplitFunds(ctx, req)
	}

	return s.moveUserToSystem(ctx, req.GetUserId(), req.GetOrderId(), req.GetMoney(), req.GetIdempotencyKey(), req.GetReason(), domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_CAPTURE, domainpb.AccountType_ACCOUNT_TYPE_HOLD)
}

func (s *Service) ReleaseFunds(ctx context.Context, req *orderpb.ReleaseFundsRequest) (*domainpb.LedgerTransaction, error) {
	return s.moveBetweenUserAccounts(ctx, req.GetUserId(), req.GetOrderId(), req.GetMoney(), req.GetIdempotencyKey(), req.GetReason(), domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_RELEASE, domainpb.AccountType_ACCOUNT_TYPE_HOLD, domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
}

func (s *Service) RefundFunds(ctx context.Context, req *orderpb.RefundFundsRequest) (*domainpb.LedgerTransaction, error) {
	return s.moveSystemToUser(ctx, req.GetUserId(), req.GetOrderId(), req.GetMoney(), req.GetIdempotencyKey(), req.GetReason(), domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_REFUND, domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
}

func (s *Service) GetAdminWallet(ctx context.Context, ownerType domainpb.WalletOwnerType, ownerID int64) (*domainpb.Wallet, error) {
	wallet, err := s.repository.EnsureWallet(ctx, ownerType, ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, err
	}
	if ownerType == domainpb.WalletOwnerType_WALLET_OWNER_TYPE_SYSTEM {
		if err := s.applySystemCommissionBalances(ctx, wallet); err != nil {
			return nil, err
		}
	}
	return wallet, nil
}

func (s *Service) applySystemCommissionBalances(ctx context.Context, wallet *domainpb.Wallet) error {
	if wallet == nil {
		return nil
	}

	balances, err := s.repository.GetSystemCommissionBalances(ctx, wallet.GetId())
	if err != nil {
		return err
	}

	for _, account := range wallet.GetAccounts() {
		if account.GetAccountType() != domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE {
			continue
		}
		account.Balance = balances[account.GetCurrencyCode()]
		if account.Balance == "" {
			account.Balance = "0"
		}
	}

	return nil
}

func (s *Service) GetAdminWalletTransactions(ctx context.Context, req *adminpb.GetWalletTransactionsRequest) ([]*domainpb.LedgerTransaction, error) {
	wallet, err := s.repository.EnsureWallet(ctx, req.GetOwnerType(), req.GetOwnerId())
	if err != nil {
		return nil, err
	}

	limit := req.GetLimit()
	if limit == 0 {
		limit = 50
	}

	var currencyCode *int64
	if req.CurrencyCode != nil {
		value := req.GetCurrencyCode()
		currencyCode = &value
	}

	return s.repository.ListWalletTransactions(ctx, wallet.Id, currencyCode, limit, req.GetOffset())
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

func (s *Service) captureSplitFunds(ctx context.Context, req *orderpb.CaptureFundsRequest) (*domainpb.LedgerTransaction, error) {
	if req.GetMoney() == nil || req.GetVendorMoney() == nil || req.GetMarketplaceFee() == nil {
		return nil, fmt.Errorf("%w: money, vendor_money and marketplace_fee are required", ErrInvalidArgument)
	}
	if req.GetVendorId() <= 0 {
		return nil, fmt.Errorf("%w: vendor_id must be greater than zero", ErrInvalidArgument)
	}
	if req.GetMoney().GetCurrencyCode() <= 0 {
		return nil, fmt.Errorf("%w: currency_code must be greater than zero", ErrInvalidArgument)
	}
	if req.GetVendorMoney().GetCurrencyCode() != req.GetMoney().GetCurrencyCode() ||
		req.GetMarketplaceFee().GetCurrencyCode() != req.GetMoney().GetCurrencyCode() {
		return nil, fmt.Errorf("%w: split currencies must match gross currency", ErrInvalidArgument)
	}

	gross, err := parsePositiveAmount(req.GetMoney().GetAmount())
	if err != nil {
		return nil, err
	}
	vendorAmount, err := parseAmount(req.GetVendorMoney().GetAmount())
	if err != nil {
		return nil, err
	}
	feeAmount, err := parseAmount(req.GetMarketplaceFee().GetAmount())
	if err != nil {
		return nil, err
	}
	if vendorAmount.Sign() < 0 || feeAmount.Sign() < 0 {
		return nil, fmt.Errorf("%w: split amounts must not be negative", ErrInvalidArgument)
	}
	if new(big.Rat).Add(vendorAmount, feeAmount).Cmp(gross) != 0 {
		return nil, fmt.Errorf("%w: gross amount must equal vendor_money plus marketplace_fee", ErrInvalidArgument)
	}

	userWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER, req.GetUserId())
	if err != nil {
		return nil, err
	}
	vendorWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_VENDOR, req.GetVendorId())
	if err != nil {
		return nil, err
	}
	systemWallet, err := s.repository.EnsureWallet(ctx, domainpb.WalletOwnerType_WALLET_OWNER_TYPE_SYSTEM, 0)
	if err != nil {
		return nil, err
	}

	userHold, err := s.repository.GetOrCreateAccount(ctx, userWallet.Id, req.GetMoney().GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_HOLD)
	if err != nil {
		return nil, err
	}
	vendorAvailable, err := s.repository.GetOrCreateAccount(ctx, vendorWallet.Id, req.GetMoney().GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, err
	}
	systemAvailable, err := s.repository.GetOrCreateAccount(ctx, systemWallet.Id, req.GetMoney().GetCurrencyCode(), domainpb.AccountType_ACCOUNT_TYPE_AVAILABLE)
	if err != nil {
		return nil, err
	}

	balance, err := s.repository.GetAccountBalance(ctx, userHold.Id)
	if err != nil {
		return nil, err
	}
	if balance.Cmp(gross) < 0 {
		return nil, ErrInsufficientFunds
	}

	entries := []repository.LedgerEntryWrite{
		{
			AccountID: userHold.Id,
			Direction: domainpb.EntryDirection_ENTRY_DIRECTION_DEBIT,
			Amount:    formatAmount(gross),
		},
	}
	if vendorAmount.Sign() > 0 {
		entries = append(entries, repository.LedgerEntryWrite{
			AccountID: vendorAvailable.Id,
			Direction: domainpb.EntryDirection_ENTRY_DIRECTION_CREDIT,
			Amount:    formatAmount(vendorAmount),
		})
	}
	if feeAmount.Sign() > 0 {
		entries = append(entries, repository.LedgerEntryWrite{
			AccountID: systemAvailable.Id,
			Direction: domainpb.EntryDirection_ENTRY_DIRECTION_CREDIT,
			Amount:    formatAmount(feeAmount),
		})
	}

	return s.postIfNotExists(ctx, repository.LedgerWriteRequest{
		IdempotencyKey: req.GetIdempotencyKey(),
		Type:           domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_CAPTURE,
		Status:         domainpb.LedgerTransactionStatus_LEDGER_TRANSACTION_STATUS_POSTED,
		Reason:         req.GetReason(),
		ReferenceType:  domainpb.ReferenceType_REFERENCE_TYPE_ORDER,
		ReferenceID:    fmt.Sprintf("%d", req.GetOrderId()),
		Entries:        entries,
		PostedAt:       time.Now(),
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
