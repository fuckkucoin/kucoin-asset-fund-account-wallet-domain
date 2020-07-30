-- 溯源活账快照漂移证明表（tsg_trace_drift_proofs）。
--
-- 设计约束：
--   - Replay 时活账摘要根与密封摘要根不一致时必须落证明；不得静默返回活账数据。
--   - proof_hash 全局唯一（SHA-256 hex）；防重复落同一证明（并发 Replay 幂等）。
--   - idem_key 唯一，防并发重复写入。
--   - drift_type 精确标记：transfers_changed | books_changed | both_changed。
--   - sealed_transfers_root / live_transfers_root：hop 摘要根对（sealed vs 重放活账）。
--   - sealed_books_root / live_books_root：book 摘要根对。
--   - entries_json 存储人类可读的漂移条目列表（可选；[]DriftEntry JSON）。
--   - 不存储原始账本余额；不存储用户资金流水明文。
--   - 一经写入不可修改（仅允许 proof_hash 查重）。
--
-- 不变量：
--   - sealed_transfers_root 与对应 tsg_trace_snapshots.transfers_root 一致。
--   - proof_hash = SHA-256(snapshot_id|drift_type|sealed_tr|live_tr|sealed_br|live_br)。
--
-- 迁移编号：0201
-- 作者：Span / Likeo / King
-- 日期：2022.01.11

CREATE TABLE IF NOT EXISTS tsg_trace_drift_proofs (
    id                      VARCHAR(36)     NOT NULL PRIMARY KEY           COMMENT 'UUID v4，trace_drift_proof_id',
    snapshot_id             VARCHAR(36)     NOT NULL DEFAULT ''            COMMENT '关联 tsg_trace_snapshots.id',
    snapshot_gen            VARCHAR(36)     NOT NULL DEFAULT ''            COMMENT '关联快照的 snapshot_gen（便于直查）',
    user_id                 VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '被调查用户 ID',
    asset                   VARCHAR(32)     NOT NULL DEFAULT ''            COMMENT '追踪资产代码',
    drift_type              VARCHAR(24)     NOT NULL DEFAULT ''            COMMENT '漂移类型：transfers_changed | books_changed | both_changed',
    sealed_transfers_root   VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '密封时 hop 摘要根（hex）',
    live_transfers_root     VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '重放时活账 hop 摘要根（hex）',
    sealed_books_root       VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '密封时 book 摘要根（hex）',
    live_books_root         VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '重放时活账 book 摘要根（hex）',
    entries_json            MEDIUMTEXT      NOT NULL                       COMMENT '漂移条目（[]DriftEntry JSON；可为空数组）',
    proof_hash              VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '证明指纹 SHA-256 hex（全局唯一索引）',
    idem_key                VARCHAR(256)    NOT NULL DEFAULT ''            COMMENT '幂等键（防并发重复写入）',
    detected_at             DATETIME(3)     NOT NULL                       COMMENT '漂移检测时刻',
    created_at              DATETIME(3)     NOT NULL                       COMMENT '行创建时刻',
    UNIQUE KEY uk_tsg_proofs_hash (proof_hash),
    UNIQUE KEY uk_tsg_proofs_idem (idem_key),
    KEY idx_tsg_proofs_snap (snapshot_id),
    KEY idx_tsg_proofs_snap_gen (snapshot_gen),
    KEY idx_tsg_proofs_user (user_id, detected_at),
    KEY idx_tsg_proofs_type (drift_type, detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='溯源活账快照漂移证明（trace_drift_proof）';
