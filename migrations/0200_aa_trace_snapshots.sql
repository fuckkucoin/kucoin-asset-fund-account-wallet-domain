-- 溯源活账快照表（tsg_trace_snapshots）。
--
-- 设计约束：
--   - TraceAsset 执行后创建 open 快照；Seal 后固化 TransfersRoot / BooksRoot / AsOf。
--   - snapshot_gen 全局唯一（UUID v4），供法医路径关联；Seal 前后不变。
--   - TransfersRoot / BooksRoot 为 SHA-256 hex；密封后不可修改。
--   - 状态机（FSM）：open → sealed → exported | superseded。
--     exported / superseded 为终态；不可退出。
--   - 同用户/同资产 Seal 新快照时，旧 sealed/open 快照置 superseded（最多一个有效密封）。
--   - idem_key 唯一，防止同一请求重复创建快照。
--   - 不存储活账实时数据；不存储账本划转金额明文（仅存 amount_hex）。
--   - hops_json / books_json 存储密封时固化的 hop 列表与 book 列表（JSON 字符串）。
--
-- 状态机（FSM）：
--   open ──(Seal)──▶ sealed
--   sealed ──(Export)──▶ exported            [终态]
--   sealed ──(Supersede)──▶ superseded       [终态]
--   open ──(Supersede)──▶ superseded         [终态]
--
-- 迁移编号：0200
-- 作者：Span / Likeo / King
-- 日期：2022.01.10

CREATE TABLE IF NOT EXISTS tsg_trace_snapshots (
    id                  VARCHAR(36)     NOT NULL PRIMARY KEY           COMMENT 'UUID v4，trace_snapshot_id',
    snapshot_gen        VARCHAR(36)     NOT NULL DEFAULT ''            COMMENT '快照世代标识符（UUID v4，全局唯一；供法医路径关联）',
    user_id             VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '被调查用户 ID',
    asset               VARCHAR(32)     NOT NULL DEFAULT ''            COMMENT '追踪资产代码（大写，如 BTC / USDT）',
    status              VARCHAR(16)     NOT NULL DEFAULT 'open'        COMMENT '状态机：open | sealed | exported | superseded',
    hops_json           MEDIUMTEXT      NOT NULL                       COMMENT '密封时固化的 hop 列表（JSON 数组；[]HopRecord）',
    books_json          MEDIUMTEXT      NOT NULL                       COMMENT '密封时固化的 book 余额列表（JSON 数组；[]BookRecord）',
    transfers_root      VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT 'hop 列表 SHA-256 摘要根（hex；sealed 后非空）',
    books_root          VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT 'book 列表 SHA-256 摘要根（hex；sealed 后非空）',
    as_of               DATETIME(3)                                    COMMENT '快照密封时刻（UTC；sealed 后非空）',
    actor               VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '执行操作的运维人员 ID',
    idem_key            VARCHAR(256)    NOT NULL DEFAULT ''            COMMENT '幂等键（防重复创建）',
    superseded_by       VARCHAR(36)     NOT NULL DEFAULT ''            COMMENT '取代本快照的新快照 ID（superseded 时非空）',
    sealed_at           DATETIME(3)                                    COMMENT '密封时刻（NULL = 未密封）',
    exported_at         DATETIME(3)                                    COMMENT '导出时刻（NULL = 未导出）',
    superseded_at       DATETIME(3)                                    COMMENT '被取代时刻（NULL = 未取代）',
    created_at          DATETIME(3)     NOT NULL                       COMMENT '行创建时刻',
    updated_at          DATETIME(3)     NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_tsg_snaps_gen (snapshot_gen),
    UNIQUE KEY uk_tsg_snaps_idem (idem_key),
    KEY idx_tsg_snaps_user_asset (user_id, asset, status),
    KEY idx_tsg_snaps_status (status, created_at),
    KEY idx_tsg_snaps_user_status (user_id, status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='溯源活账快照世代（trace_snapshot）';
