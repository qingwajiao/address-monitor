CREATE TABLE IF NOT EXISTS refresh_tokens
(
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL,
    token_hash VARCHAR(128)    NOT NULL UNIQUE,
    expires_at DATETIME        NOT NULL,
    revoked    TINYINT         NOT NULL DEFAULT 0 COMMENT '0=有效 1=已撤销',
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_token_hash (token_hash)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;