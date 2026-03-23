package evm

// BSC 复用 EVMListener，通过 config.dev.yaml 中的 type=evm 自动路由
// ETH 和 BSC 共用同一个 factory，通过 name 和 chain_id 区分
// 此文件仅作说明用途，无需额外代码
