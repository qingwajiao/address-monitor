CREATE TABLE IF NOT EXISTS chain_sync_status
(
    id          INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    chain       VARCHAR(16)     NOT NULL,
    instance_id VARCHAR(64)     NOT NULL,
    last_block  BIGINT UNSIGNED NOT NULL DEFAULT 0,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_chain_instance (chain, instance_id)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;