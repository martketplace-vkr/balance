alter table balance.ledger_transaction
    alter column reference_type type smallint
    using reference_type::smallint,
    alter column reference_type set not null;
