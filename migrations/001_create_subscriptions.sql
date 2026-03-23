CREATE TABLE IF NOT EXISTS subscriptions (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id      VARCHAR(64)  NOT NULL,
    chain        VARCHAR(16)  NOT NULL,
    address      VARCHAR(128) NOT NULL COMMENT '统一小写存储',
    label        VARCHAR(128) DEFAULT '',
    callback_url VARCHAR(512) NOT NULL,
    secret       VARCHAR(128) NOT NULL,
    status       TINYINT NOT NULL DEFAULT 1,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_chain_address_user (chain, address, user_id),
    INDEX idx_chain_address (chain, address),
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
