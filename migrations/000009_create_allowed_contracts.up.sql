CREATE TABLE IF NOT EXISTS allowed_contracts
(
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    chain            VARCHAR(16)  NOT NULL COMMENT '链名称，大写，如 ETH / BSC / TRON',
    contract_address VARCHAR(128) NOT NULL COMMENT '合约地址，小写存储',
    symbol           VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '代币符号，仅作备注，如 USDT',
    enabled          TINYINT      NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用',
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_chain_contract (chain, contract_address)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COMMENT = '系统级合约地址白名单，为空表示该链不限制合约';
