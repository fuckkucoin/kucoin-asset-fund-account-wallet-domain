-- 资金科目开/关/冻状态。重启后以本表为准，不再只留在进程内存。
--
-- 作者：Cason
-- 迁移编号：0226

CREATE TABLE IF NOT EXISTS aa_book_life (
  user_id VARCHAR(64) NOT NULL,
  book VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (user_id, book)
);
