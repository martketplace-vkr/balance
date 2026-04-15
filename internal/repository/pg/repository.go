package pg

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"

	"github.com/jmoiron/sqlx"

	domainpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/domain"
)

type Repository struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetDepositAddressByUser(ctx context.Context, userID int64, network string) (*depositAddressRow, error) {
	var row depositAddressRow
	query := `
		select id, user_id, address, network, created_at, updated_at
		from balance.deposit_address
		where user_id = $1
			and network = $2
	`
	if err := r.db.GetContext(ctx, &row, query, userID, network); err != nil {
		return nil, err
	}

	return &row, nil
}

func (r *Repository) SaveDepositAddress(ctx context.Context, userID int64, address, network string) (*domainpb.DepositAddress, bool, error) {
	if existing, err := r.GetDepositAddressByUser(ctx, userID, network); err == nil {
		if existing.Address == address {
			return toDepositAddress(*existing), false, nil
		}
	} else if err != sql.ErrNoRows {
		return nil, false, err
	}

	var row depositAddressRow
	query := `
		insert into balance.deposit_address(user_id, address, network)
		values ($1, $2, $3)
		on conflict (user_id) do update
		set address = excluded.address,
			network = excluded.network,
			updated_at = now()
		returning id, user_id, address, network, created_at, updated_at
	`
	if err := r.db.GetContext(ctx, &row, query, userID, address, network); err != nil {
		return nil, false, err
	}

	return toDepositAddress(row), true, nil
}

func (r *Repository) ListDepositAddressesByUser(ctx context.Context, userID int64) ([]*domainpb.DepositAddress, error) {
	rows := make([]depositAddressRow, 0)
	query := `
		select id, user_id, address, network, created_at, updated_at
		from balance.deposit_address
		where user_id = $1
		order by created_at
	`
	if err := r.db.SelectContext(ctx, &rows, query, userID); err != nil {
		return nil, err
	}

	addresses := make([]*domainpb.DepositAddress, 0, len(rows))
	for _, row := range rows {
		addresses = append(addresses, toDepositAddress(row))
	}

	return addresses, nil
}

func (r *Repository) AssignDepositAddress(ctx context.Context, userID int64, network string, pool []string) (string, error) {
	if existing, err := r.GetDepositAddressByUser(ctx, userID, network); err == nil {
		return existing.Address, nil
	} else if err != sql.ErrNoRows {
		return "", err
	}

	query := `
		insert into balance.deposit_address(user_id, address, network)
		values ($1, $2, $3)
		on conflict do nothing
		returning address
	`

	for _, candidate := range pool {
		var address string
		err := r.db.GetContext(ctx, &address, query, userID, candidate, network)
		if err == nil {
			return address, nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
	}

	if existing, err := r.GetDepositAddressByUser(ctx, userID, network); err == nil {
		return existing.Address, nil
	} else if err != sql.ErrNoRows {
		return "", err
	}

	return "", fmt.Errorf("no free deposit addresses")
}

func (r *Repository) GetWalletByOwner(ctx context.Context, ownerType domainpb.WalletOwnerType, ownerID int64) (*domainpb.Wallet, error) {
	var row walletRow
	query := `
		select id, owner_type, owner_id, status, created_at, updated_at
		from balance.wallet
		where owner_type = $1 and owner_id = $2
	`
	if err := r.db.GetContext(ctx, &row, query, int32(ownerType), ownerID); err != nil {
		return nil, err
	}

	return r.GetWalletByID(ctx, row.ID)
}

func (r *Repository) EnsureWallet(ctx context.Context, ownerType domainpb.WalletOwnerType, ownerID int64) (*domainpb.Wallet, error) {
	query := `
		insert into balance.wallet(owner_type, owner_id, status)
		values ($1, $2, $3)
		on conflict (owner_type, owner_id) do update
		set updated_at = now()
		returning id, owner_type, owner_id, status, created_at, updated_at
	`

	var row walletRow
	if err := r.db.GetContext(ctx, &row, query, int32(ownerType), ownerID, int32(domainpb.WalletStatus_WALLET_STATUS_ACTIVE)); err != nil {
		return nil, err
	}

	return r.GetWalletByID(ctx, row.ID)
}

func (r *Repository) GetWalletByID(ctx context.Context, walletID int64) (*domainpb.Wallet, error) {
	var row walletRow
	query := `
		select id, owner_type, owner_id, status, created_at, updated_at
		from balance.wallet
		where id = $1
	`
	if err := r.db.GetContext(ctx, &row, query, walletID); err != nil {
		return nil, err
	}

	accounts, err := r.getWalletAccounts(ctx, walletID)
	if err != nil {
		return nil, err
	}

	return toWallet(row, accounts), nil
}

func (r *Repository) GetOrCreateAccount(
	ctx context.Context,
	walletID int64,
	currencyCode int64,
	accountType domainpb.AccountType,
) (*domainpb.Account, error) {
	query := `
		insert into balance.account(wallet_id, currency_code, account_type, status)
		values ($1, $2, $3, $4)
		on conflict (wallet_id, currency_code, account_type) do update
		set updated_at = now()
		returning id, wallet_id, currency_code, account_type, status, created_at, updated_at
	`

	var row accountRow
	if err := r.db.GetContext(
		ctx,
		&row,
		query,
		walletID,
		currencyCode,
		int32(accountType),
		int32(domainpb.AccountStatus_ACCOUNT_STATUS_ACTIVE),
	); err != nil {
		return nil, err
	}

	balance, err := r.getAccountBalance(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	row.Balance = balance

	return toAccount(row), nil
}

func (r *Repository) GetAccountBalance(ctx context.Context, accountID int64) (*big.Rat, error) {
	raw, err := r.getAccountBalance(ctx, accountID)
	if err != nil {
		return nil, err
	}

	value, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("invalid numeric balance: %s", raw)
	}

	return value, nil
}

func (r *Repository) getAccountBalance(ctx context.Context, accountID int64) (string, error) {
	var balance string
	query := `
		select coalesce(
			sum(case when direction = $2 then amount else -amount end),
			0
		)::text as balance
		from balance.entry
		where account_id = $1
	`
	if err := r.db.GetContext(ctx, &balance, query, accountID, int32(domainpb.EntryDirection_ENTRY_DIRECTION_CREDIT)); err != nil {
		return "", err
	}

	return balance, nil
}

func (r *Repository) getWalletAccounts(ctx context.Context, walletID int64) ([]*domainpb.Account, error) {
	rows := make([]accountRow, 0)
	query := `
		select
			a.id,
			a.wallet_id,
			a.currency_code,
			a.account_type,
			a.status,
			coalesce(sum(case when e.direction = 2 then e.amount else -e.amount end), 0)::text as balance,
			a.created_at,
			a.updated_at
		from balance.account a
		left join balance.entry e on e.account_id = a.id
		where a.wallet_id = $1
		group by a.id, a.wallet_id, a.currency_code, a.account_type, a.status, a.created_at, a.updated_at
		order by a.currency_code, a.account_type
	`
	if err := r.db.SelectContext(ctx, &rows, query, walletID); err != nil {
		return nil, err
	}

	accounts := make([]*domainpb.Account, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, toAccount(row))
	}

	return accounts, nil
}
