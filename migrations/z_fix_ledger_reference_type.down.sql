alter table balance.ledger_transaction
    alter column reference_type drop not null,
    alter column reference_type type text
    using reference_type::text;
