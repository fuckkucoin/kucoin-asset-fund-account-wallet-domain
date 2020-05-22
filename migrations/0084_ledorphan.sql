-- 账本批次孤儿分录发现与封堵引擎 — 数据库表定义。
--
-- 新增六张表：
--   lo_batch_gens       — 批次世代锁（续提互斥 CAS 记录）
--   lo_orphan_reports   — 孤儿报告（每次扫描产出一份）
--   lo_orphan_items     — 孤儿分录明细（每条一行）
--   lo_seal_actions     — 封堵/冲正/忽略动作证明
--   lo_integrity_proofs — 批次终态完整性证明
--   lo_idem_blocks      — 幂等拒写记录（封堵后防续写）
--
-- 设计原则：
--   lo_batch_gens.fence_epoch 唯一约束实现世代 CAS 防双 worker；
--   lo_idem_blocks.idem_key 唯一约束为封堵后的最后防线；
--   金额列保留 DECIMAL(36,18)；时间列 DATETIME(3)（精度 1ms）。
--
-- 不改写任何既有 aa_ 表；表前缀 lo_ 与既有 aa_/fx_ 隔离。
--
-- 内部主机占位（不修改，保持产品契约不变）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16）
--
-- 作者：Kinsley
-- 日期：2020.12.26

-- ────────────────────────────────────────────────────────────────────────────
-- lo_batch_gens: 批次世代锁
-- ────────────────────────────────────────────────────────────────────────────
-- 世代 CAS 协议：
--   Acquire: UPDATE SET generation=generation+1, worker_id=?, status='locked',
--            locked_at=NOW(), expires_at=NOW()+90s WHERE batch_id=? AND status IN ('open','expired','released')
--            OR INSERT 首次写入。
--   Refresh: UPDATE SET expires_at=NOW()+90s WHERE batch_id=? AND worker_id=? AND generation=?
--   Release: UPDATE SET status='released', released_at=NOW() WHERE batch_id=? AND worker_id=? AND generation=?
--
--   fence_epoch 由应用层管理（persist.KV 的 Fence 机制）；此列仅供 SQL 层审计。

CREATE TABLE IF NOT EXISTS lo_batch_gens (
  id             VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '行 ID（UUID，应用层生成）',

  batch_id       VARCHAR(64)    NOT NULL
    COMMENT '所属 ledbatch 批次 ID',

  generation     BIGINT         NOT NULL DEFAULT 1
    COMMENT '当前世代号（从 1 开始，每次 Acquire 时原子递增）',

  worker_id      VARCHAR(255)   NOT NULL DEFAULT ''
    COMMENT '持有锁的 worker 标识（hostname:pid:uuid）',

  status         VARCHAR(16)    NOT NULL DEFAULT 'open'
    COMMENT '锁状态：open / locked / expired / released',

  locked_at      DATETIME(3)    NULL
    COMMENT '本次 CAS 成功时刻',

  expires_at     DATETIME(3)    NULL
    COMMENT 'worker 必须在此时刻前续期；过期后其他 worker 可抢锁',

  released_at    DATETIME(3)    NULL
    COMMENT 'worker 主动释放时刻（仅 released 状态有值）',

  fence_epoch    BIGINT         NOT NULL DEFAULT 0
    COMMENT 'KV 围栏纪元（与 persist.Fence.Epoch 对齐，供审计）',

  created_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='批次续提世代锁；CAS 原子控制，防双 worker 并行 Submit 同批次';

CREATE UNIQUE INDEX IF NOT EXISTS uq_lo_batch_gens_batch
  ON lo_batch_gens (batch_id);

CREATE INDEX IF NOT EXISTS idx_lo_batch_gens_status
  ON lo_batch_gens (status, expires_at);

-- ────────────────────────────────────────────────────────────────────────────
-- lo_orphan_reports: 孤儿报告
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS lo_orphan_reports (
  report_id      VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '报告 UUID',

  batch_id       VARCHAR(64)    NOT NULL
    COMMENT '所属批次 ID',

  generation     BIGINT         NOT NULL DEFAULT 0
    COMMENT '扫描时对应的世代号（0 = 扫描阶段未持锁）',

  status         VARCHAR(16)    NOT NULL DEFAULT 'open'
    COMMENT '报告状态：open / closed / superseded',

  total_orphans  INT            NOT NULL DEFAULT 0
    COMMENT '孤儿总数',

  hanging_count  INT            NOT NULL DEFAULT 0
    COMMENT 'OrphanHanging（batch 标记 submitted 但 ledger 无记录）数量',

  missing_count  INT            NOT NULL DEFAULT 0
    COMMENT 'OrphanMissing（ledger 已落账但 batch 仍 pending）数量',

  intent_hash    VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '意图集合 idem keys 的 SHA-256（用于不可变性绑定）',

  ledger_hash    VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '已落账 hop idem keys 的 SHA-256',

  fence_epoch    BIGINT         NOT NULL DEFAULT 0
    COMMENT 'KV 围栏纪元（供审计）',

  created_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='批次孤儿检测报告；每次崩溃重启扫描产出一份，旧报告标为 superseded';

CREATE INDEX IF NOT EXISTS idx_lo_orphan_reports_batch
  ON lo_orphan_reports (batch_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lo_orphan_reports_status
  ON lo_orphan_reports (status, created_at DESC);

-- ────────────────────────────────────────────────────────────────────────────
-- lo_orphan_items: 孤儿分录明细
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS lo_orphan_items (
  orphan_id      VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '孤儿记录 UUID',

  batch_id       VARCHAR(64)    NOT NULL
    COMMENT '所属批次 ID',

  report_id      VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '所属孤儿报告 ID',

  seq            INT            NOT NULL DEFAULT 0
    COMMENT '在原批次 Items 中的顺序号（1-based）',

  user_id        VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '账户 ID',

  asset          VARCHAR(32)    NOT NULL DEFAULT ''
    COMMENT '资产符号',

  amount         DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT '金额',

  direction      VARCHAR(32)    NOT NULL DEFAULT ''
    COMMENT 'hop 方向（与 ledbatch.Item.Direction 一致）',

  idem_key       VARCHAR(191)   NOT NULL DEFAULT ''
    COMMENT '原 hop 幂等键',

  kind           VARCHAR(16)    NOT NULL DEFAULT 'hanging'
    COMMENT '孤儿类型：hanging（批次标 submitted 但 ledger 无）/ missing（ledger 有但批次 pending）',

  status         VARCHAR(16)    NOT NULL DEFAULT 'detected'
    COMMENT '处置状态：detected / sealed / reversed / ignored',

  ledger_id      VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'ledger entry ID（hanging 类型有值）',

  batch_ledger_id VARCHAR(64)   NOT NULL DEFAULT ''
    COMMENT '批次记录的 LedgerID（missing 场景下可能有值）',

  note           VARCHAR(512)   NOT NULL DEFAULT ''
    COMMENT '人工备注或处置原因',

  detected_at    DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  sealed_at      DATETIME(3)    NULL,
  reversed_at    DATETIME(3)    NULL,

  fence_epoch    BIGINT         NOT NULL DEFAULT 0
    COMMENT 'KV 围栏纪元',

  updated_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='孤儿分录明细；每条对应批次中一个未正确落账的 hop';

CREATE INDEX IF NOT EXISTS idx_lo_orphan_items_batch
  ON lo_orphan_items (batch_id, detected_at DESC);

CREATE INDEX IF NOT EXISTS idx_lo_orphan_items_report
  ON lo_orphan_items (report_id);

CREATE INDEX IF NOT EXISTS idx_lo_orphan_items_status
  ON lo_orphan_items (status, detected_at DESC);

CREATE INDEX IF NOT EXISTS idx_lo_orphan_items_idem
  ON lo_orphan_items (idem_key);

-- ────────────────────────────────────────────────────────────────────────────
-- lo_seal_actions: 封堵/冲正/忽略动作证明
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS lo_seal_actions (
  seal_id        VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '封堵动作 UUID',

  batch_id       VARCHAR(64)    NOT NULL
    COMMENT '所属批次',

  report_id      VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '所属孤儿报告',

  kind           VARCHAR(16)    NOT NULL DEFAULT 'block'
    COMMENT '动作类型：block（纯封堵）/ reverse（冲正）/ ignore（忽略）',

  status         VARCHAR(16)    NOT NULL DEFAULT 'pending'
    COMMENT '动作状态：pending / applied / failed / aborted',

  orphan_ids     TEXT           NOT NULL DEFAULT ''
    COMMENT '本次处置的孤儿 ID 列表（JSON 数组）',

  actor1         VARCHAR(128)   NOT NULL DEFAULT ''
    COMMENT '发起人',

  actor2         VARCHAR(128)   NOT NULL DEFAULT ''
    COMMENT '复核人（与 actor1 不同，双人授权）',

  approved_at    DATETIME(3)    NULL
    COMMENT '双人授权完成时刻',

  applied_at     DATETIME(3)    NULL
    COMMENT '执行完成时刻',

  failure_reason VARCHAR(256)   NOT NULL DEFAULT ''
    COMMENT '执行失败原因码',

  reverse_entry_ids TEXT        NOT NULL DEFAULT ''
    COMMENT '冲正动作产出的 ledger entry ID 列表（JSON 数组，仅 reverse 有值）',

  proof_hash     VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '动作完成后绑定的完整性哈希',

  fence_epoch    BIGINT         NOT NULL DEFAULT 0,

  created_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='封堵/冲正/忽略动作证明；不可篡改，供审计和事故复盘';

CREATE INDEX IF NOT EXISTS idx_lo_seal_actions_batch
  ON lo_seal_actions (batch_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lo_seal_actions_report
  ON lo_seal_actions (report_id);

CREATE INDEX IF NOT EXISTS idx_lo_seal_actions_status
  ON lo_seal_actions (status);

-- ────────────────────────────────────────────────────────────────────────────
-- lo_integrity_proofs: 批次终态完整性证明
-- ────────────────────────────────────────────────────────────────────────────
-- 哈希算法：SHA-256(sorted(intentIdem) || 0x00 || sorted(ledgerIdem) || 0x00 || sorted(sealIdem))
-- 审计方仅需三组 idem key 列表即可独立复算，无需访问账本。

CREATE TABLE IF NOT EXISTS lo_integrity_proofs (
  proof_id       VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '证明 UUID',

  batch_id       VARCHAR(64)    NOT NULL
    COMMENT '所属批次',

  report_id      VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '关联孤儿报告（无孤儿时为空）',

  seal_id        VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '关联封堵动作（无封堵时为空）',

  generation     BIGINT         NOT NULL DEFAULT 0
    COMMENT '生成证明时的世代号',

  status         VARCHAR(16)    NOT NULL DEFAULT 'pending'
    COMMENT '证明状态：pending / verified / invalid',

  hash           VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '三路 SHA-256 完整性哈希（64 字符 hex）',

  intent_count   INT            NOT NULL DEFAULT 0
    COMMENT '意图集合大小',

  ledger_count   INT            NOT NULL DEFAULT 0
    COMMENT '已落账 hop 数量',

  seal_count     INT            NOT NULL DEFAULT 0
    COMMENT '封堵集合大小',

  intent_idem_keys  MEDIUMTEXT  NOT NULL DEFAULT ''
    COMMENT '意图集合 idem keys（JSON 数组，已排序）',

  ledger_idem_keys  MEDIUMTEXT  NOT NULL DEFAULT ''
    COMMENT '落账集合 idem keys（JSON 数组，已排序）',

  seal_idem_keys    MEDIUMTEXT  NOT NULL DEFAULT ''
    COMMENT '封堵集合 idem keys（JSON 数组，已排序）',

  verified_at    DATETIME(3)    NULL
    COMMENT '最后校验时刻',

  fence_epoch    BIGINT         NOT NULL DEFAULT 0,

  created_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='批次终态完整性证明；哈希=意图集合⊕落账集合⊕封堵集合';

CREATE INDEX IF NOT EXISTS idx_lo_integrity_proofs_batch
  ON lo_integrity_proofs (batch_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lo_integrity_proofs_status
  ON lo_integrity_proofs (status, created_at DESC);

-- ────────────────────────────────────────────────────────────────────────────
-- lo_idem_blocks: 幂等拒写记录
-- ────────────────────────────────────────────────────────────────────────────
-- 封堵后，idem_key 列入此表；ledbatch.Submit 在提交 hop 前须先检查此表。
-- idem_key 唯一约束是封堵后防续写的数据库层最终防线（与应用层双重防护）。

CREATE TABLE IF NOT EXISTS lo_idem_blocks (
  idem_key       VARCHAR(191)   NOT NULL PRIMARY KEY
    COMMENT '被封堵的幂等键（全局唯一）',

  batch_id       VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '来源批次',

  orphan_id      VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '对应孤儿记录 ID',

  seal_id        VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '关联封堵动作 ID',

  blocked_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '写入时刻'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='幂等拒写记录；封堵后 idem_key 列入此表，ledbatch.Submit 必须拒绝同 idem 续写';

CREATE INDEX IF NOT EXISTS idx_lo_idem_blocks_batch
  ON lo_idem_blocks (batch_id);

CREATE INDEX IF NOT EXISTS idx_lo_idem_blocks_seal
  ON lo_idem_blocks (seal_id);
