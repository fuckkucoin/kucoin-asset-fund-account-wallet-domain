-- 出金批次部分失败残留与续提互斥收口：数据表迁移
--
-- 包含表：wd_residues, wd_resume_leases, wd_release_proofs, wd_batch_closeouts
-- 作者：Kinsley
--
-- 设计约束：
--   - 所有主键使用 UUID（VARCHAR 36），避免全局序列依赖。
--   - wd_residues.wd_batch_id 与 wdbatch 逻辑外键（不加 FOREIGN KEY，避免跨库约束）。
--   - wd_release_proofs 只追加，历史记录永久保留供审计。
--   - wd_residues (wd_batch_id, seq, idem_key) 有联合唯一约束防重复探测。
--   - wd_resume_leases (wd_batch_id) 有唯一约束：同一批次只保留一条租约记录。
--   - wd_batch_closeouts (wd_batch_id) 唯一：一个批次只能有一条收口单。
--   - 证明表含 checksum 字段（SHA-256），防篡改可验证。

-- ─── wd_residues ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wd_residues (
    id                  VARCHAR(36)     NOT NULL,
    wd_batch_id         VARCHAR(36)     NOT NULL,           -- 来源 wdbatch 批次 ID（必须是 wdbatch，非 ledbatch）
    seq                 INT             NOT NULL,            -- 在原批次中的序号
    user_id             VARCHAR(64)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    amount              VARCHAR(64)     NOT NULL,           -- 字符串精度
    chain               VARCHAR(16)     NOT NULL DEFAULT '',
    to_address          VARCHAR(256)    NOT NULL DEFAULT '',
    idem_key            VARCHAR(191)    NOT NULL,           -- 原批次 item 幂等键
    origin              VARCHAR(16)     NOT NULL,           -- failed / skipped
    fail_reason         VARCHAR(512)    NOT NULL DEFAULT '',
    status              VARCHAR(16)     NOT NULL,           -- open/released/retried/escalated
    freeze_occupied     TINYINT(1)      NOT NULL DEFAULT 0, -- 最后一次观察：冻结占用是否存在
    ledger_occupied     TINYINT(1)      NOT NULL DEFAULT 0, -- 最后一次观察：账本占用是否存在
    occupy_ledger_id    VARCHAR(36)     NOT NULL DEFAULT '', -- 账本侧记录 ID（若有）
    proof_id            VARCHAR(36)     NOT NULL DEFAULT '', -- 关联 wd_release_proofs.id
    retry_batch_id      VARCHAR(36)     NOT NULL DEFAULT '', -- 重提新批次 ID
    escalate_ledger_id  VARCHAR(36)     NOT NULL DEFAULT '', -- 升级挂账账本 ID
    detected_at         DATETIME(3)     NOT NULL,
    resolved_at         DATETIME(3)              DEFAULT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_wd_residues_batch (wd_batch_id),
    INDEX idx_wd_residues_user (user_id),
    INDEX idx_wd_residues_status (status),
    INDEX idx_wd_residues_origin (origin),
    INDEX idx_wd_residues_detected_at (detected_at),
    UNIQUE INDEX uniq_wd_residues_batch_seq_idem (wd_batch_id, seq, idem_key)
        COMMENT '同批次同 seq+idemKey 只允许一条残留记录（幂等探测）'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='出金批次部分失败/跳过残留项（每笔对应原 wdbatch item 中的 failed/skipped）';

-- ─── wd_resume_leases ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wd_resume_leases (
    id              VARCHAR(36)     NOT NULL,
    wd_batch_id     VARCHAR(36)     NOT NULL,           -- 对应批次 ID
    holder          VARCHAR(128)    NOT NULL,           -- 持有者标识（worker ID / operator ID）
    gen             BIGINT          NOT NULL DEFAULT 1,  -- 世代号，单调递增
    acquired_at     DATETIME(3)     NOT NULL,
    expires_at      DATETIME(3)     NOT NULL,
    released_at     DATETIME(3)              DEFAULT NULL,
    released        TINYINT(1)      NOT NULL DEFAULT 0, -- 是否已主动释放
    PRIMARY KEY (id),
    UNIQUE INDEX uniq_wd_resume_lease_batch (wd_batch_id)
        COMMENT '同一批次只保留一条租约记录（Acquire 时 UPSERT）',
    INDEX idx_wd_resume_leases_holder (holder),
    INDEX idx_wd_resume_leases_expires_at (expires_at),
    INDEX idx_wd_resume_leases_gen (gen)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='出金批次 Resume 续提互斥世代锁；同批次同时只能有一个活跃持有者';

-- ─── wd_release_proofs ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wd_release_proofs (
    id              VARCHAR(36)     NOT NULL,
    wd_batch_id     VARCHAR(36)     NOT NULL,
    residue_id      VARCHAR(36)     NOT NULL,           -- 关联 wd_residues.id
    user_id         VARCHAR(64)     NOT NULL,
    asset           VARCHAR(16)     NOT NULL,
    amount          VARCHAR(64)     NOT NULL,
    kind            VARCHAR(32)     NOT NULL,           -- freeze_release/ledger_cancel/escalate_susp/no_occupy
    actor           VARCHAR(128)    NOT NULL,
    ledger_ref      VARCHAR(36)     NOT NULL DEFAULT '', -- 账本操作参考 ID
    summary         TEXT            NOT NULL,
    checksum        VARCHAR(64)     NOT NULL,           -- SHA-256(kind|wd_batch_id|residue_id|user_id|asset|amount|actor)
    lease_gen       BIGINT          NOT NULL DEFAULT 0, -- 操作时持有的 ResumeLease 世代号
    recorded_at     DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    UNIQUE INDEX uniq_wd_release_proof_residue (residue_id)
        COMMENT '每条残留项只允许一条释放证明（防二次操作）',
    INDEX idx_wd_release_proofs_batch (wd_batch_id),
    INDEX idx_wd_release_proofs_user (user_id),
    INDEX idx_wd_release_proofs_kind (kind),
    INDEX idx_wd_release_proofs_recorded_at (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='出金批次残留项占用释放/升级证明（只追加，供 ops 与审计只读）';

-- ─── wd_batch_closeouts ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wd_batch_closeouts (
    id                  VARCHAR(36)     NOT NULL,
    wd_batch_id         VARCHAR(36)     NOT NULL,           -- 来源 wdbatch 批次 ID
    status              VARCHAR(16)     NOT NULL,           -- closed / escalated
    requested           VARCHAR(64)     NOT NULL,           -- 原批次总请求额
    submitted           VARCHAR(64)     NOT NULL,           -- 已提交额
    skipped_released    VARCHAR(64)     NOT NULL DEFAULT '0', -- 跳过项已释放量
    failed_released     VARCHAR(64)     NOT NULL DEFAULT '0', -- 失败项已释放量
    escalated_amount    VARCHAR(64)     NOT NULL DEFAULT '0', -- 升级挂账量
    conserve_ok         TINYINT(1)      NOT NULL DEFAULT 0, -- Conserve 等式是否通过
    conserve_note       TEXT            NOT NULL,           -- Conserve 详情（不平衡时有说明）
    proof_count         INT             NOT NULL DEFAULT 0, -- 关联证明数量
    residue_count       INT             NOT NULL DEFAULT 0, -- 残留项数量
    actor               VARCHAR(128)    NOT NULL,
    closed_at           DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    UNIQUE INDEX uniq_wd_batch_closeout_batch (wd_batch_id)
        COMMENT '同一批次只允许一条收口单（不可覆盖）',
    INDEX idx_wd_batch_closeouts_status (status),
    INDEX idx_wd_batch_closeouts_closed_at (closed_at),
    INDEX idx_wd_batch_closeouts_actor (actor)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='出金批次收口单（残留全部处置、Conserve 通过后写入；不可变快照）';
