-- version: v2
CREATE TABLE orders (
  id BIGINT NOT NULL,
  amount_cents DOUBLE NULL RENAMED_FROM amount,
  status VARCHAR(16) NOT NULL,
  created_at DATE NULL
);
