package pg

import (
	"database/sql"
	"time"

	domainpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type walletRow struct {
	ID        int64     `db:"id"`
	OwnerType int32     `db:"owner_type"`
	OwnerID   int64     `db:"owner_id"`
	Status    int32     `db:"status"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type accountRow struct {
	ID           int64     `db:"id"`
	WalletID     int64     `db:"wallet_id"`
	CurrencyCode int64     `db:"currency_code"`
	AccountType  int32     `db:"account_type"`
	Status       int32     `db:"status"`
	Balance      string    `db:"balance"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type ledgerTransactionRow struct {
	ID             int64          `db:"id"`
	IdempotencyKey string         `db:"idempotency_key"`
	Type           int32          `db:"transaction_type"`
	Status         int32          `db:"status"`
	Reason         sql.NullString `db:"reason"`
	ReferenceType  int32          `db:"reference_type"`
	ReferenceID    sql.NullString `db:"reference_id"`
	CreatedAt      time.Time      `db:"created_at"`
	PostedAt       sql.NullTime   `db:"posted_at"`
	ReversedAt     sql.NullTime   `db:"reversed_at"`
}

type entryRow struct {
	ID            int64     `db:"id"`
	TransactionID int64     `db:"transaction_id"`
	AccountID     int64     `db:"account_id"`
	Direction     int32     `db:"direction"`
	Amount        string    `db:"amount"`
	CreatedAt     time.Time `db:"created_at"`
	CurrencyCode  int64     `db:"currency_code"`
}

type topUpRow struct {
	ID             int64          `db:"id"`
	WalletID       int64          `db:"wallet_id"`
	CurrencyCode   int64          `db:"currency_code"`
	Amount         string         `db:"amount"`
	ProviderType   int32          `db:"provider_type"`
	ProviderName   string         `db:"provider_name"`
	Status         int32          `db:"status"`
	ExternalID     sql.NullString `db:"external_id"`
	ExternalStatus sql.NullString `db:"external_status"`
	PaymentURL     sql.NullString `db:"payment_url"`
	WalletAddress  sql.NullString `db:"wallet_address"`
	WalletTag      sql.NullString `db:"wallet_tag"`
	Network        sql.NullString `db:"network"`
	TxHash         sql.NullString `db:"tx_hash"`
	IdempotencyKey string         `db:"idempotency_key"`
	ExpiresAt      sql.NullTime   `db:"expires_at"`
	PaidAt         sql.NullTime   `db:"paid_at"`
	ConfirmedAt    sql.NullTime   `db:"confirmed_at"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
}

type withdrawalRow struct {
	ID              int64          `db:"id"`
	WalletID        int64          `db:"wallet_id"`
	CurrencyCode    int64          `db:"currency_code"`
	Amount          string         `db:"amount"`
	DestinationType int32          `db:"destination_type"`
	Destination     string         `db:"destination"`
	Status          int32          `db:"status"`
	ExternalID      sql.NullString `db:"external_id"`
	IdempotencyKey  string         `db:"idempotency_key"`
	RequestedAt     time.Time      `db:"requested_at"`
	ProcessedAt     sql.NullTime   `db:"processed_at"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
}

type LedgerWriteRequest struct {
	IdempotencyKey  string
	Type            domainpb.LedgerTransactionType
	Status          domainpb.LedgerTransactionStatus
	Reason          string
	ReferenceType   domainpb.ReferenceType
	ReferenceID     string
	DebitAccountID  int64
	CreditAccountID int64
	Amount          string
	PostedAt        time.Time
}

type TopUpCreateRequest struct {
	WalletID       int64
	CurrencyCode   int64
	Amount         string
	ProviderType   domainpb.ProviderType
	ProviderName   string
	Status         domainpb.TopUpStatus
	ExternalID     string
	ExternalStatus string
	PaymentURL     string
	WalletAddress  string
	WalletTag      string
	Network        string
	TxHash         string
	IdempotencyKey string
	ExpiresAt      *time.Time
	PaidAt         *time.Time
	ConfirmedAt    *time.Time
}

type WithdrawalCreateRequest struct {
	WalletID        int64
	CurrencyCode    int64
	Amount          string
	DestinationType domainpb.DestinationType
	Destination     string
	Status          domainpb.WithdrawalStatus
	ExternalID      string
	IdempotencyKey  string
}

func toWallet(row walletRow, accounts []*domainpb.Account) *domainpb.Wallet {
	return &domainpb.Wallet{
		Id:        row.ID,
		OwnerType: domainpb.WalletOwnerType(row.OwnerType),
		OwnerId:   row.OwnerID,
		Status:    domainpb.WalletStatus(row.Status),
		Accounts:  accounts,
		CreatedAt: timestamppb.New(row.CreatedAt),
		UpdatedAt: timestamppb.New(row.UpdatedAt),
	}
}

func toAccount(row accountRow) *domainpb.Account {
	return &domainpb.Account{
		Id:           row.ID,
		WalletId:     row.WalletID,
		CurrencyCode: row.CurrencyCode,
		AccountType:  domainpb.AccountType(row.AccountType),
		Status:       domainpb.AccountStatus(row.Status),
		Balance:      row.Balance,
		CreatedAt:    timestamppb.New(row.CreatedAt),
		UpdatedAt:    timestamppb.New(row.UpdatedAt),
	}
}

func toLedgerTransaction(row ledgerTransactionRow) *domainpb.LedgerTransaction {
	tx := &domainpb.LedgerTransaction{
		Id:             row.ID,
		IdempotencyKey: row.IdempotencyKey,
		Type:           domainpb.LedgerTransactionType(row.Type),
		Status:         domainpb.LedgerTransactionStatus(row.Status),
		Reason:         row.Reason.String,
		ReferenceType:  domainpb.ReferenceType(row.ReferenceType),
		ReferenceId:    row.ReferenceID.String,
		CreatedAt:      timestamppb.New(row.CreatedAt),
	}
	if row.PostedAt.Valid {
		tx.PostedAt = timestamppb.New(row.PostedAt.Time)
	}
	if row.ReversedAt.Valid {
		tx.ReversedAt = timestamppb.New(row.ReversedAt.Time)
	}

	return tx
}

func toLedgerEntry(row entryRow) *domainpb.LedgerEntry {
	return &domainpb.LedgerEntry{
		Id:        row.ID,
		AccountId: row.AccountID,
		Direction: domainpb.EntryDirection(row.Direction),
		Money: &domainpb.Money{
			Amount:       row.Amount,
			CurrencyCode: row.CurrencyCode,
		},
		CreatedAt: timestamppb.New(row.CreatedAt),
	}
}

func toTopUp(row topUpRow) *domainpb.TopUp {
	topUp := &domainpb.TopUp{
		Id:             row.ID,
		WalletId:       row.WalletID,
		Money:          &domainpb.Money{Amount: row.Amount, CurrencyCode: row.CurrencyCode},
		ProviderType:   domainpb.ProviderType(row.ProviderType),
		ProviderName:   row.ProviderName,
		Status:         domainpb.TopUpStatus(row.Status),
		ExternalId:     row.ExternalID.String,
		ExternalStatus: row.ExternalStatus.String,
		PaymentUrl:     row.PaymentURL.String,
		WalletAddress:  row.WalletAddress.String,
		WalletTag:      row.WalletTag.String,
		Network:        row.Network.String,
		TxHash:         row.TxHash.String,
		IdempotencyKey: row.IdempotencyKey,
		CreatedAt:      timestamppb.New(row.CreatedAt),
		UpdatedAt:      timestamppb.New(row.UpdatedAt),
	}
	if row.ExpiresAt.Valid {
		topUp.ExpiresAt = timestamppb.New(row.ExpiresAt.Time)
	}
	if row.PaidAt.Valid {
		topUp.PaidAt = timestamppb.New(row.PaidAt.Time)
	}
	if row.ConfirmedAt.Valid {
		topUp.ConfirmedAt = timestamppb.New(row.ConfirmedAt.Time)
	}

	return topUp
}

func toWithdrawal(row withdrawalRow) *domainpb.Withdrawal {
	withdrawal := &domainpb.Withdrawal{
		Id:              row.ID,
		WalletId:        row.WalletID,
		Money:           &domainpb.Money{Amount: row.Amount, CurrencyCode: row.CurrencyCode},
		DestinationType: domainpb.DestinationType(row.DestinationType),
		Destination:     row.Destination,
		Status:          domainpb.WithdrawalStatus(row.Status),
		ExternalId:      row.ExternalID.String,
		IdempotencyKey:  row.IdempotencyKey,
		RequestedAt:     timestamppb.New(row.RequestedAt),
		CreatedAt:       timestamppb.New(row.CreatedAt),
		UpdatedAt:       timestamppb.New(row.UpdatedAt),
	}
	if row.ProcessedAt.Valid {
		withdrawal.ProcessedAt = timestamppb.New(row.ProcessedAt.Time)
	}

	return withdrawal
}
