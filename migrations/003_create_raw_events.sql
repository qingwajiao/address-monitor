CREATE TABLE IF NOT EXISTS raw_events (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    chain        VARCHAR(16)  NOT NULL,
    tx_hash      VARCHAR(128) NOT NULL,
    block_number BIGINT UNSIGNED NOT NULL,
    block_time   INT UNSIGNED NOT NULL,
    event_type   VARCHAR(64)  NOT NULL,
    raw_data     JSON         NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_chain_time (chain, created_at),
    INDEX idx_tx_hash (tx_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
