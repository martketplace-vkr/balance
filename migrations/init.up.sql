create schema if not exists balance;

create table if not exists balance.wallet (
    id bigserial primary key,
    owner_type smallint not null,
    owner_id bigint not null,
    status smallint not null default 1,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (owner_type, owner_id)
);

create table if not exists balance.account (
    id bigserial primary key,
    wallet_id bigint not null references balance.wallet(id) on delete cascade,
    currency_code bigint not null,
    account_type smallint not null,
    status smallint not null default 1,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (wallet_id, currency_code, account_type)
);

create index if not exists idx_balance_account_wallet_id
    on balance.account(wallet_id);

create table if not exists balance.ledger_transaction (
    id bigserial primary key,
    idempotency_key text not null,
    transaction_type smallint not null,
    status smallint not null,
    reason text,
    reference_type text,
    reference_id text,
    metadata jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    posted_at timestamptz,
    reversed_at timestamptz,
    unique (idempotency_key)
);

create index if not exists idx_balance_ledger_transaction_reference
    on balance.ledger_transaction(reference_type, reference_id);

create table if not exists balance.entry (
    id bigserial primary key,
    transaction_id bigint not null references balance.ledger_transaction(id) on delete cascade,
    account_id bigint not null references balance.account(id),
    direction smallint not null,
    amount numeric(24, 8) not null check (amount > 0),
    created_at timestamptz not null default now()
);

create index if not exists idx_balance_entry_transaction_id
    on balance.entry(transaction_id);

create index if not exists idx_balance_entry_account_id
    on balance.entry(account_id);

create table if not exists balance.top_up (
    id bigserial primary key,
    wallet_id bigint not null references balance.wallet(id) on delete cascade,
    currency_code bigint not null,
    amount numeric(24, 8) not null check (amount > 0),
    provider_type smallint not null,
    provider_name text not null,
    status smallint not null,
    external_id text,
    external_status text,
    payment_url text,
    wallet_address text,
    wallet_tag text,
    network text,
    tx_hash text,
    idempotency_key text not null,
    metadata jsonb not null default '{}'::jsonb,
    expires_at timestamptz,
    paid_at timestamptz,
    confirmed_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (idempotency_key),
    unique (provider_type, provider_name, external_id),
    unique (provider_type, tx_hash)
);

create index if not exists idx_balance_top_up_wallet_id
    on balance.top_up(wallet_id);

create index if not exists idx_balance_top_up_status
    on balance.top_up(status);

create table if not exists balance.withdrawal (
    id bigserial primary key,
    wallet_id bigint not null references balance.wallet(id) on delete cascade,
    currency_code bigint not null,
    amount numeric(24, 8) not null check (amount > 0),
    destination_type smallint not null,
    destination text not null,
    status smallint not null,
    external_id text,
    idempotency_key text not null,
    metadata jsonb not null default '{}'::jsonb,
    requested_at timestamptz not null default now(),
    processed_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (idempotency_key)
);

create index if not exists idx_balance_withdrawal_wallet_id
    on balance.withdrawal(wallet_id);
