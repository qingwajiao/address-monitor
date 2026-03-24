CREATE TABLE IF NOT EXISTS watched_addresses
(
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    app_id     BIGINT UNSIGNED NOT NULL,
    chain      VARCHAR(16)     NOT NULL,
    address    VARCHAR(128)    NOT NULL COMMENT '统一小写存储',
    label      VARCHAR(128)    NOT NULL DEFAULT '',
    status     TINYINT         NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用',
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_app_chain_address (app_id, chain, address),
    INDEX idx_chain_address (chain, address),
    INDEX idx_app_id (app_id)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;