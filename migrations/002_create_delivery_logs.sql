CREATE TABLE IF NOT EXISTS delivery_logs (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    event_id        VARCHAR(128) NOT NULL,
    subscription_id BIGINT UNSIGNED NOT NULL,
    chain           VARCHAR(16)  NOT NULL,
    tx_hash         VARCHAR(128) NOT NULL,
    event_type      VARCHAR(64)  NOT NULL,
    payload         JSON         NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
    retry_count     INT NOT NULL DEFAULT 0,
    callback_url    VARCHAR(512) NOT NULL,
    response_code   INT NULL,
    response_body   TEXT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_event_sub (event_id, subscription_id),
    INDEX idx_status (status),
    INDEX idx_tx_hash (tx_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
