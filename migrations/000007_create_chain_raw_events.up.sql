CREATE TABLE IF NOT EXISTS chain_raw_events_eth
(
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tx_hash      VARCHAR(128)    NOT NULL,
    block_number BIGINT UNSIGNED NOT NULL,
    block_time   INT UNSIGNED    NOT NULL,
    event_type   VARCHAR(64)     NOT NULL,
    raw_data     JSON            NOT NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_tx_hash (tx_hash),
    INDEX idx_created_at (created_at)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS chain_raw_events_bsc
(
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tx_hash      VARCHAR(128)    NOT NULL,
    block_number BIGINT UNSIGNED NOT NULL,
    block_time   INT UNSIGNED    NOT NULL,
    event_type   VARCHAR(64)     NOT NULL,
    raw_data     JSON            NOT NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_tx_hash (tx_hash),
    INDEX idx_created_at (created_at)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS chain_raw_events_tron
(
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tx_hash      VARCHAR(128)    NOT NULL,
    block_number BIGINT UNSIGNED NOT NULL,
    block_time   INT UNSIGNED    NOT NULL,
    event_type   VARCHAR(64)     NOT NULL,
    raw_data     JSON            NOT NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_tx_hash (tx_hash),
    INDEX idx_created_at (created_at)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS chain_raw_events_sol
(
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tx_hash      VARCHAR(128)    NOT NULL,
    block_number BIGINT UNSIGNED NOT NULL,
    block_time   INT UNSIGNED    NOT NULL,
    event_type   VARCHAR(64)     NOT NULL,
    raw_data     JSON            NOT NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_tx_hash (tx_hash),
    INDEX idx_created_at (created_at)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;