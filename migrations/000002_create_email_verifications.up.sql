CREATE TABLE IF NOT EXISTS email_verifications
(
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL,
    token      VARCHAR(64)     NOT NULL UNIQUE,
    expires_at DATETIME        NOT NULL,
    used       TINYINT         NOT NULL DEFAULT 0 COMMENT '0=未使用 1=已使用',
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_token (token),
    INDEX idx_user_id (user_id)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;