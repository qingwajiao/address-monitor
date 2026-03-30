ALTER TABLE apps
    ADD COLUMN allowed_contracts JSON NULL DEFAULT NULL COMMENT 'App 级合约白名单，格式 {"ETH":["0xabc..."],"TRON":["TR7..."]}，为 NULL 表示不限制';
