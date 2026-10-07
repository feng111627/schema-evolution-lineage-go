-- version: v1
CREATE TABLE orders (
  id BIGINT NOT NULL,
  amount BIGINT NULL,
  status VARCHAR(16) NULL,
  legacy_note TEXT NULL
);
