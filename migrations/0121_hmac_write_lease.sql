-- 功能：HMAC 验签写路径租约绑定漂移封堵引擎——数据库表结构。
-- 重点：
--   hmac_write_leases 记录每次 VerifyAndLease 签发的写路径租约，
--     state 按 FSM 单调流转（issued→bound→consumed/expired/revoked）。
--   hmac_nonce_seen 为跨节点、跨重启的持久化 nonce 防重放仓，
--     expire_at 用于 NonceCleanWorker 周期清理；禁止手动删改 seen_id 行。
--   hmac_write_proofs 为只增漂移证明表，记录 WriteCheck 拒绝事件；禁止删改。
--   hmac_halt_generations 跟踪 Halt 翻转世代号，防止旧租约在 Halt 后通过 WriteCheck。
-- 风险点：
--   lease_id、proof_id、seen_id 均为 UUID，VARCHAR(64) 足够；禁止缩短。
--   expire_at / issued_at 等时间列用 DATETIME(3) UTC 存储，出库后须 .UTC() 转换。
--   halted 列用 TINYINT(1) 存储布尔值（MySQL 无原生 BOOL 类型）。
--   hmac_write_proofs 仅用 INSERT IGNORE / ON CONFLICT DO NOTHING 写入，严禁 UPDATE。
-- Author: TianpengLan
-- 作者：Axel
-- Date: 2021-03-15

CREATE TABLE IF NOT EXISTS hmac_halt_generations (
  gen_id       VARCHAR(128)  NOT NULL,
  generation   BIGINT        NOT NULL DEFAULT 0,
  halted       TINYINT(1)    NOT NULL DEFAULT 0,
  updated_at   DATETIME(3)   NOT NULL,
  PRIMARY KEY (gen_id)
);

CREATE TABLE IF NOT EXISTS hmac_write_leases (
  lease_id     VARCHAR(64)   NOT NULL,
  key_id       VARCHAR(128)  NOT NULL,
  nonce        VARCHAR(128)  NOT NULL,
  ts           BIGINT        NOT NULL DEFAULT 0,
  method_path  VARCHAR(255)  NOT NULL DEFAULT '',
  halt_gen     BIGINT        NOT NULL DEFAULT 0,
  state        VARCHAR(16)   NOT NULL DEFAULT 'issued',
  issued_at    DATETIME(3)   NOT NULL,
  expire_at    DATETIME(3)   NOT NULL,
  bound_at     DATETIME(3)   NULL,
  consumed_at  DATETIME(3)   NULL,
  revoked_at   DATETIME(3)   NULL,
  revoke_note  VARCHAR(255)  NOT NULL DEFAULT '',
  PRIMARY KEY (lease_id),
  KEY idx_hwl_key_nonce (key_id, nonce),
  KEY idx_hwl_state_expire (state, expire_at),
  KEY idx_hwl_halt_gen_state (halt_gen, state)
);

CREATE TABLE IF NOT EXISTS hmac_nonce_seen (
  seen_id      VARCHAR(64)   NOT NULL,
  key_id       VARCHAR(128)  NOT NULL,
  nonce        VARCHAR(128)  NOT NULL,
  lease_id     VARCHAR(64)   NOT NULL DEFAULT '',
  seen_at      DATETIME(3)   NOT NULL,
  expire_at    DATETIME(3)   NOT NULL,
  PRIMARY KEY (seen_id),
  KEY idx_hns_expire_at (expire_at),
  KEY idx_hns_key_nonce (key_id, nonce)
);

CREATE TABLE IF NOT EXISTS hmac_write_proofs (
  proof_id     VARCHAR(64)   NOT NULL,
  lease_id     VARCHAR(64)   NOT NULL DEFAULT '',
  key_id       VARCHAR(128)  NOT NULL DEFAULT '',
  nonce        VARCHAR(128)  NOT NULL DEFAULT '',
  write_scope  VARCHAR(64)   NOT NULL DEFAULT '',
  reason       VARCHAR(32)   NOT NULL,
  detail       VARCHAR(512)  NOT NULL DEFAULT '',
  attempted_at DATETIME(3)   NOT NULL,
  halt_gen     BIGINT        NOT NULL DEFAULT 0,
  digest       VARCHAR(64)   NOT NULL DEFAULT '',
  PRIMARY KEY (proof_id),
  KEY idx_hwp_lease_id (lease_id),
  KEY idx_hwp_reason_at (reason, attempted_at),
  KEY idx_hwp_key_id_at (key_id, attempted_at)
);
