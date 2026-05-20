package pg

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	domainpb "github.com/martketplace-vkr/balance/pkg/api/grpc/v1/domain"
)

func (r *Repository) GetTopUpByUser(ctx context.Context, userID, topUpID int64) (*domainpb.TopUp, error) {
	var row topUpRow
	query := `
		select
			t.id, t.wallet_id, t.currency_code, t.amount::text as amount, t.provider_type, t.provider_name, t.status,
			t.external_id, t.external_status, t.payment_url, t.wallet_address, t.wallet_tag, t.network, t.tx_hash,
			t.idempotency_key, t.expires_at, t.paid_at, t.confirmed_at, t.created_at, t.updated_at
		from balance.top_up t
		join balance.wallet w on w.id = t.wallet_id
		where w.owner_type = $1 and w.owner_id = $2 and t.id = $3
	`
	if err := r.db.GetContext(ctx, &row, query, int32(domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER), userID, topUpID); err != nil {
		return nil, err
	}

	return toTopUp(row), nil
}

func (r *Repository) ListTopUpsByUser(ctx context.Context, userID int64, limit uint32, offset uint64) ([]*domainpb.TopUp, error) {
	rows := make([]topUpRow, 0)
	query := `
		select
			t.id, t.wallet_id, t.currency_code, t.amount::text as amount, t.provider_type, t.provider_name, t.status,
			t.external_id, t.external_status, t.payment_url, t.wallet_address, t.wallet_tag, t.network, t.tx_hash,
			t.idempotency_key, t.expires_at, t.paid_at, t.confirmed_at, t.created_at, t.updated_at
		from balance.top_up t
		join balance.wallet w on w.id = t.wallet_id
		where w.owner_type = $1 and w.owner_id = $2
		order by t.id desc
		limit $3 offset $4
	`
	if err := r.db.SelectContext(ctx, &rows, query, int32(domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER), userID, int64(limit), int64(offset)); err != nil {
		return nil, err
	}

	result := make([]*domainpb.TopUp, 0, len(rows))
	for _, row := range rows {
		result = append(result, toTopUp(row))
	}

	return result, nil
}

func (r *Repository) ListTopUps(ctx context.Context, filter TopUpListFilter) ([]*domainpb.TopUp, error) {
	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}

	var currencyCode any
	if filter.CurrencyCode != nil {
		currencyCode = *filter.CurrencyCode
	}
	var providerType any
	if filter.ProviderType != nil {
		providerType = int32(*filter.ProviderType)
	}
	var status any
	if filter.Status != nil {
		status = int32(*filter.Status)
	}

	rows := make([]topUpRow, 0)
	query := `
		select
			t.id, t.wallet_id, t.currency_code, t.amount::text as amount, t.provider_type, t.provider_name, t.status,
			t.external_id, t.external_status, t.payment_url, t.wallet_address, t.wallet_tag, t.network, t.tx_hash,
			t.idempotency_key, t.expires_at, t.paid_at, t.confirmed_at, t.created_at, t.updated_at
		from balance.top_up t
		where ($1::bigint is null or t.currency_code = $1)
			and ($2::smallint is null or t.provider_type = $2)
			and ($3::smallint is null or t.status = $3)
		order by t.id desc
		limit $4 offset $5
	`
	if err := r.db.SelectContext(ctx, &rows, query, currencyCode, providerType, status, int64(limit), int64(filter.Offset)); err != nil {
		return nil, err
	}

	result := make([]*domainpb.TopUp, 0, len(rows))
	for _, row := range rows {
		result = append(result, toTopUp(row))
	}

	return result, nil
}

func (r *Repository) GetTopUpByProviderExternalID(ctx context.Context, providerType domainpb.ProviderType, providerName, externalID string) (*domainpb.TopUp, error) {
	var row topUpRow
	query := `
		select
			t.id, t.wallet_id, t.currency_code, t.amount::text as amount, t.provider_type, t.provider_name, t.status,
			t.external_id, t.external_status, t.payment_url, t.wallet_address, t.wallet_tag, t.network, t.tx_hash,
			t.idempotency_key, t.expires_at, t.paid_at, t.confirmed_at, t.created_at, t.updated_at
		from balance.top_up t
		where t.provider_type = $1 and t.provider_name = $2 and t.external_id = $3
	`
	if err := r.db.GetContext(ctx, &row, query, int32(providerType), providerName, externalID); err != nil {
		return nil, err
	}

	return toTopUp(row), nil
}

func (r *Repository) CreateTopUp(ctx context.Context, req TopUpCreateRequest) (*domainpb.TopUp, error) {
	query := `
		insert into balance.top_up(
			wallet_id, currency_code, amount, provider_type, provider_name, status,
			external_id, external_status, payment_url, wallet_address, wallet_tag, network, tx_hash,
			idempotency_key, expires_at, paid_at, confirmed_at
		)
		values (
			$1, $2, $3, $4, $5, $6,
			nullif($7, ''), nullif($8, ''), nullif($9, ''), nullif($10, ''), nullif($11, ''), nullif($12, ''), nullif($13, ''),
			$14, $15, $16, $17
		)
		returning
			id, wallet_id, currency_code, amount::text as amount, provider_type, provider_name, status,
			external_id, external_status, payment_url, wallet_address, wallet_tag, network, tx_hash,
			idempotency_key, expires_at, paid_at, confirmed_at, created_at, updated_at
	`

	var row topUpRow
	if err := r.db.GetContext(
		ctx,
		&row,
		query,
		req.WalletID,
		req.CurrencyCode,
		req.Amount,
		int32(req.ProviderType),
		req.ProviderName,
		int32(req.Status),
		req.ExternalID,
		req.ExternalStatus,
		req.PaymentURL,
		req.WalletAddress,
		req.WalletTag,
		req.Network,
		req.TxHash,
		req.IdempotencyKey,
		req.ExpiresAt,
		req.PaidAt,
		req.ConfirmedAt,
	); err != nil {
		return nil, err
	}

	return toTopUp(row), nil
}

func (r *Repository) ConfirmTopUp(ctx context.Context, req TopUpConfirmRequest) (*domainpb.TopUp, *domainpb.LedgerTransaction, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var topUpRow topUpRow
	lockQuery := `
		select
			id, wallet_id, currency_code, amount::text as amount, provider_type, provider_name, status,
			external_id, external_status, payment_url, wallet_address, wallet_tag, network, tx_hash,
			idempotency_key, expires_at, paid_at, confirmed_at, created_at, updated_at
		from balance.top_up
		where id = $1
		for update
	`
	if err := tx.GetContext(ctx, &topUpRow, lockQuery, req.TopUpID); err != nil {
		return nil, nil, err
	}

	transactionID, err := r.getOrCreateLedgerTransactionInTx(ctx, tx, LedgerWriteRequest{
		IdempotencyKey:  req.IdempotencyKey,
		Type:            domainpb.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TOP_UP,
		Status:          domainpb.LedgerTransactionStatus_LEDGER_TRANSACTION_STATUS_POSTED,
		Reason:          req.Reason,
		ReferenceType:   domainpb.ReferenceType_REFERENCE_TYPE_TOP_UP,
		ReferenceID:     topUpRow.ExternalID.String,
		DebitAccountID:  req.DebitAccountID,
		CreditAccountID: req.CreditAccountID,
		Amount:          req.Amount,
		PostedAt:        time.Now(),
	})
	if err != nil {
		return nil, nil, err
	}

	updateQuery := `
		update balance.top_up
		set
			status = $2,
			external_status = nullif($3, ''),
			paid_at = coalesce(paid_at, now()),
			confirmed_at = coalesce(confirmed_at, now()),
			updated_at = now()
		where id = $1
		returning
			id, wallet_id, currency_code, amount::text as amount, provider_type, provider_name, status,
			external_id, external_status, payment_url, wallet_address, wallet_tag, network, tx_hash,
			idempotency_key, expires_at, paid_at, confirmed_at, created_at, updated_at
	`
	if err := tx.GetContext(ctx, &topUpRow, updateQuery, req.TopUpID, int32(domainpb.TopUpStatus_TOP_UP_STATUS_CONFIRMED), req.ExternalStatus); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	transaction, err := r.GetTransaction(ctx, transactionID)
	if err != nil {
		return nil, nil, err
	}

	return toTopUp(topUpRow), transaction, nil
}

func (r *Repository) GetWithdrawalByUser(ctx context.Context, userID, withdrawalID int64) (*domainpb.Withdrawal, error) {
	var row withdrawalRow
	query := `
		select
			d.id, d.wallet_id, d.currency_code, d.amount::text as amount, d.destination_type, d.destination,
			d.status, d.external_id, d.idempotency_key, d.requested_at, d.processed_at, d.created_at, d.updated_at
		from balance.withdrawal d
		join balance.wallet w on w.id = d.wallet_id
		where w.owner_type = $1 and w.owner_id = $2 and d.id = $3
	`
	if err := r.db.GetContext(ctx, &row, query, int32(domainpb.WalletOwnerType_WALLET_OWNER_TYPE_USER), userID, withdrawalID); err != nil {
		return nil, err
	}

	return toWithdrawal(row), nil
}

func (r *Repository) CreateWithdrawal(ctx context.Context, req WithdrawalCreateRequest) (*domainpb.Withdrawal, error) {
	query := `
		insert into balance.withdrawal(
			wallet_id, currency_code, amount, destination_type, destination, status, external_id, idempotency_key
		)
		values ($1, $2, $3, $4, $5, $6, nullif($7, ''), $8)
		returning
			id, wallet_id, currency_code, amount::text as amount, destination_type, destination,
			status, external_id, idempotency_key, requested_at, processed_at, created_at, updated_at
	`

	var row withdrawalRow
	if err := r.db.GetContext(
		ctx,
		&row,
		query,
		req.WalletID,
		req.CurrencyCode,
		req.Amount,
		int32(req.DestinationType),
		req.Destination,
		int32(req.Status),
		req.ExternalID,
		req.IdempotencyKey,
	); err != nil {
		return nil, err
	}

	return toWithdrawal(row), nil
}

func (r *Repository) GetTransaction(ctx context.Context, transactionID int64) (*domainpb.LedgerTransaction, error) {
	var row ledgerTransactionRow
	query := `
		select
			id, idempotency_key, transaction_type, status, reason, reference_type, reference_id,
			created_at, posted_at, reversed_at
		from balance.ledger_transaction
		where id = $1
	`
	if err := r.db.GetContext(ctx, &row, query, transactionID); err != nil {
		return nil, err
	}

	transactions, err := r.loadTransactionsWithEntries(ctx, []ledgerTransactionRow{row})
	if err != nil {
		return nil, err
	}

	return transactions[0], nil
}

func (r *Repository) GetTransactionByIdempotencyKey(ctx context.Context, key string) (*domainpb.LedgerTransaction, error) {
	var row ledgerTransactionRow
	query := `
		select
			id, idempotency_key, transaction_type, status, reason, reference_type, reference_id,
			created_at, posted_at, reversed_at
		from balance.ledger_transaction
		where idempotency_key = $1
	`
	if err := r.db.GetContext(ctx, &row, query, key); err != nil {
		return nil, err
	}

	transactions, err := r.loadTransactionsWithEntries(ctx, []ledgerTransactionRow{row})
	if err != nil {
		return nil, err
	}

	return transactions[0], nil
}

func (r *Repository) ListWalletTransactions(
	ctx context.Context,
	walletID int64,
	currencyCode *int64,
	limit uint32,
	offset uint64,
) ([]*domainpb.LedgerTransaction, error) {
	rows := make([]ledgerTransactionRow, 0)
	query := `
		select distinct
			lt.id, lt.idempotency_key, lt.transaction_type, lt.status, lt.reason, lt.reference_type,
			lt.reference_id, lt.created_at, lt.posted_at, lt.reversed_at
		from balance.ledger_transaction lt
		join balance.entry e on e.transaction_id = lt.id
		join balance.account a on a.id = e.account_id
		where a.wallet_id = $1 and ($2::bigint is null or a.currency_code = $2)
		order by lt.id desc
		limit $3 offset $4
	`

	var filter any
	if currencyCode != nil {
		filter = *currencyCode
	}
	if err := r.db.SelectContext(ctx, &rows, query, walletID, filter, int64(limit), int64(offset)); err != nil {
		return nil, err
	}

	return r.loadTransactionsWithEntries(ctx, rows)
}

func (r *Repository) loadTransactionsWithEntries(ctx context.Context, rows []ledgerTransactionRow) ([]*domainpb.LedgerTransaction, error) {
	if len(rows) == 0 {
		return []*domainpb.LedgerTransaction{}, nil
	}

	ids := make([]int64, 0, len(rows))
	indexByID := make(map[int64]int, len(rows))
	result := make([]*domainpb.LedgerTransaction, 0, len(rows))

	for idx, row := range rows {
		ids = append(ids, row.ID)
		indexByID[row.ID] = idx
		result = append(result, toLedgerTransaction(row))
	}

	query, args, err := sqlx.In(`
		select
			e.id, e.transaction_id, e.account_id, e.direction, e.amount::text as amount, e.created_at, a.currency_code
		from balance.entry e
		join balance.account a on a.id = e.account_id
		where e.transaction_id in (?)
		order by e.id
	`, ids)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)

	entryRows := make([]entryRow, 0)
	if err := r.db.SelectContext(ctx, &entryRows, query, args...); err != nil {
		return nil, err
	}

	for _, row := range entryRows {
		idx := indexByID[row.TransactionID]
		result[idx].Entries = append(result[idx].Entries, toLedgerEntry(row))
	}

	return result, nil
}

func (r *Repository) PostLedgerTransaction(ctx context.Context, req LedgerWriteRequest) (*domainpb.LedgerTransaction, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var transactionID int64
	insertTx := `
		insert into balance.ledger_transaction(
			idempotency_key, transaction_type, status, reason, reference_type, reference_id, posted_at
		)
		values ($1, $2, $3, nullif($4, ''), $5, nullif($6, ''), $7)
		returning id
	`
	if err := tx.GetContext(
		ctx,
		&transactionID,
		insertTx,
		req.IdempotencyKey,
		int32(req.Type),
		int32(req.Status),
		req.Reason,
		int32(req.ReferenceType),
		req.ReferenceID,
		req.PostedAt,
	); err != nil {
		return nil, err
	}

	insertEntry := `
		insert into balance.entry(transaction_id, account_id, direction, amount)
		values ($1, $2, $3, $4)
	`
	if _, err := tx.ExecContext(ctx, insertEntry, transactionID, req.DebitAccountID, int32(domainpb.EntryDirection_ENTRY_DIRECTION_DEBIT), req.Amount); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, insertEntry, transactionID, req.CreditAccountID, int32(domainpb.EntryDirection_ENTRY_DIRECTION_CREDIT), req.Amount); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetTransaction(ctx, transactionID)
}

func (r *Repository) getOrCreateLedgerTransactionInTx(ctx context.Context, tx *sqlx.Tx, req LedgerWriteRequest) (int64, error) {
	var existingID int64
	if err := tx.GetContext(ctx, &existingID, `select id from balance.ledger_transaction where idempotency_key = $1`, req.IdempotencyKey); err == nil {
		return existingID, nil
	} else if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	insertTx := `
		insert into balance.ledger_transaction(
			idempotency_key, transaction_type, status, reason, reference_type, reference_id, posted_at
		)
		values ($1, $2, $3, nullif($4, ''), $5, nullif($6, ''), $7)
		returning id
	`
	if err := tx.GetContext(
		ctx,
		&existingID,
		insertTx,
		req.IdempotencyKey,
		int32(req.Type),
		int32(req.Status),
		req.Reason,
		int32(req.ReferenceType),
		req.ReferenceID,
		req.PostedAt,
	); err != nil {
		return 0, err
	}

	insertEntry := `
		insert into balance.entry(transaction_id, account_id, direction, amount)
		values ($1, $2, $3, $4)
	`
	if _, err := tx.ExecContext(ctx, insertEntry, existingID, req.DebitAccountID, int32(domainpb.EntryDirection_ENTRY_DIRECTION_DEBIT), req.Amount); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, insertEntry, existingID, req.CreditAccountID, int32(domainpb.EntryDirection_ENTRY_DIRECTION_CREDIT), req.Amount); err != nil {
		return 0, err
	}

	return existingID, nil
}
