CREATE TABLE IF NOT EXISTS apps
(
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id      BIGINT UNSIGNED NOT NULL,
    name         VARCHAR(64)     NOT NULL,
    api_key      VARCHAR(64)     NOT NULL UNIQUE,
    secret       VARCHAR(64)     NOT NULL COMMENT '应用级 HMAC 签名密钥',
    callback_url VARCHAR(512)    NOT NULL DEFAULT '' COMMENT '应用级全局回调地址',
    status       TINYINT         NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用',
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_api_key (api_key)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;