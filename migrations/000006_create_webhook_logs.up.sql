CREATE TABLE IF NOT EXISTS webhook_logs
(
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    event_id      VARCHAR(128)    NOT NULL COMMENT '全局幂等键',
    app_id        BIGINT UNSIGNED NOT NULL,
    address_id    BIGINT UNSIGNED NOT NULL COMMENT '关联 watched_addresses.id',
    chain         VARCHAR(16)     NOT NULL,
    tx_hash       VARCHAR(128)    NOT NULL,
    event_type    VARCHAR(64)     NOT NULL,
    payload       JSON            NOT NULL,
    status        VARCHAR(16)     NOT NULL DEFAULT 'pending'
        COMMENT 'pending/success/failed/dead',
    retry_count   INT             NOT NULL DEFAULT 0,
    callback_url  VARCHAR(512)    NOT NULL COMMENT '推送时快照的回调地址',
    response_code INT             NULL,
    response_body TEXT            NULL,
    created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id, created_at),
    UNIQUE KEY uk_event_address (event_id, address_id, created_at),
    INDEX idx_app_id (app_id),
    INDEX idx_status (status),
    INDEX idx_tx_hash (tx_hash),
    INDEX idx_created_at (created_at)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
    PARTITION BY RANGE (TO_DAYS(created_at)) (
        PARTITION p202601 VALUES LESS THAN (TO_DAYS('2026-02-01')),
        PARTITION p202602 VALUES LESS THAN (TO_DAYS('2026-03-01')),
        PARTITION p202603 VALUES LESS THAN (TO_DAYS('2026-04-01')),
        PARTITION p202604 VALUES LESS THAN (TO_DAYS('2026-05-01')),
        PARTITION p202605 VALUES LESS THAN (TO_DAYS('2026-06-01')),
        PARTITION p202606 VALUES LESS THAN (TO_DAYS('2026-07-01')),
        PARTITION p202607 VALUES LESS THAN (TO_DAYS('2026-08-01')),
        PARTITION p202608 VALUES LESS THAN (TO_DAYS('2026-09-01')),
        PARTITION p202609 VALUES LESS THAN (TO_DAYS('2026-10-01')),
        PARTITION p202610 VALUES LESS THAN (TO_DAYS('2026-11-01')),
        PARTITION p202611 VALUES LESS THAN (TO_DAYS('2026-12-01')),
        PARTITION p202612 VALUES LESS THAN (TO_DAYS('2027-01-01')),
        PARTITION p_future VALUES LESS THAN MAXVALUE
        );