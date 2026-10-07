# orders 契约 v2
schema: orders
version: v2
column: id bigint not null
column: amount_cents double null renamed_from=amount
column: status varchar(16) not null
column: created_at date null
