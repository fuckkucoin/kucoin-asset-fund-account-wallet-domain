package persist

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SchemaVersionDDL Postgres 形态。ApplyOnce 先建此表再按代次落盘。

const SchemaVersionDDL = `
CREATE TABLE IF NOT EXISTS aa_schema_version (
  version INT NOT NULL PRIMARY KEY,
  name VARCHAR(191) NOT NULL UNIQUE,
  checksum VARCHAR(64) NOT NULL DEFAULT '',
  applied_at TIMESTAMPTZ NOT NULL
);
`

// SchemaVersionMySQL 与 SchemaSQL 同列。
const SchemaVersionMySQL = `
CREATE TABLE IF NOT EXISTS aa_schema_version (
  version INT NOT NULL PRIMARY KEY,
  name VARCHAR(191) NOT NULL,
  checksum VARCHAR(64) NOT NULL DEFAULT '',
  applied_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_schema_name (name)
);
`

type MigrationFile struct {
	Version int
	Name    string
	Path     string
	Checksum string
	Body string
}

type SchemaVer struct {
	Version int
	Name string
	Checksum  string
	AppliedAt time.Time
}

// ParseMigrationName 认 NNNN_name.sql。前缀必须是十进制代次号。

  func ParseMigrationName(name string) (int, error) {
	   name = strings.TrimSpace(name)
	   if !strings.HasSuffix(strings.ToLower(name), ".sql") {
		   return 0, fmt.Errorf("%w: %s", ErrMigration, name)
	  }
	   base := filepath.Base(name)
	  i := strings.IndexByte(base, '_')
	  if i <= 0 {
		   i = strings.IndexByte(base, '.')
	   }
	   if i <= 0 {
		   return 0, fmt.Errorf("%w: %s", ErrMigration, name)
	  }
	  n, err := strconv.Atoi(base[:i])
	   if err != nil || n <= 0 {
		  return 0, fmt.Errorf("%w: %s", ErrMigration, name)
	   }
	   return n, nil
   }

   func ChecksumBytes(raw []byte) string {
	  sum := sha256.Sum256(raw)
	  return hex.EncodeToString(sum[:])
   }

// ListMigrationFiles 按代次号排序。同代次两文件视为冲突。
  func ListMigrationFiles(dir string) ([]MigrationFile, error) {
	   if strings.TrimSpace(dir) == "" {
		   dir = MigrationDir()
	   }
	  ents, err := os.ReadDir(dir)
	  if err != nil {
		   return nil, err
	   }
	   out := make([]MigrationFile, 0, len(ents))
	   seen := map[int]string{}
	  for _, e := range ents {
		  if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			  continue
		  }
		   ver,
			   err := ParseMigrationName(e.Name())
		   if err != nil {
			   return nil, err
		   }
		  if prev, ok := seen[ver]; ok {
			   return nil, fmt.Errorf("%w: version %d %s vs %s", ErrMigration, ver, prev, e.Name())
		   }
		  seen[ver] = e.Name()
		   p := filepath.Join(dir, e.Name())
		  raw, err := os.ReadFile(p)
		  if err != nil {
			  return nil, err
		  }
		   out = append(out, MigrationFile{
			  Version: ver, Name: e.Name(), Path: p,
			  Checksum: ChecksumBytes(raw), Body: string(raw),
		   })
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	  if len(out) == 0 {
		   return nil, fmt.Errorf("%w: no sql in %s", ErrMigration, dir)
	  }
	   return out, nil
   }

   func EnsureSchemaVersion(ctx context.Context, db *sql.DB, d Dialect) error {
	   if db == nil {
		   return ErrTxClosed
	   }
	  ddl := SchemaVersionMySQL
	   if d == Postgres {
		  ddl = SchemaVersionDDL
	  }
	   return ApplyStatements(ctx, db, Rewrite(ddl, d))
   }

   func (s *SQLDB) ListSchemaVersions(ctx context.Context) ([]SchemaVer, error) {
	   if s == nil || s.DB == nil {
		   return nil, ErrTxClosed
	  }
	   rows,
		  err := s.Conn(ctx).QueryContext(ctx,
		  `SELECT version, name, checksum, applied_at FROM aa_schema_version ORDER BY version`)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]SchemaVer, 0, 8)
	   for rows.Next() {
		   var v SchemaVer
		   if err := rows.Scan(&v.Version, &v.Name, &v.Checksum, &v.AppliedAt); err != nil {
			  return nil, err
		  }
		  out = append(out, v)
	   }
	   return out, rows.Err()
  }

   func (s *SQLDB) CurrentSchemaVersion(ctx context.Context) (int, error) {
	  rows, err := s.ListSchemaVersions(ctx)
	   if err != nil {
		   return 0, err
	   }
	   if len(rows) == 0 {
		  return 0, nil
	   }
	  return rows[len(rows)-1].Version, nil
   }

  func (s *SQLDB) recordSchemaVersion(ctx context.Context, m MigrationFile) error {
	   q := s.Conn(ctx)
	   if s.Dialect == Postgres {
		   _, err := q.ExecContext(ctx,
			  `INSERT INTO aa_schema_version (version, name, checksum, applied_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (version) DO NOTHING`,
			   m.Version, m.Name, m.Checksum)
		  return err
	  }
	  _, err := q.ExecContext(ctx,
		   `INSERT IGNORE INTO aa_schema_version (version, name, checksum, applied_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP(3))`,
		   m.Version, m.Name, m.Checksum)
	   return err
  }

// AppliedMap 已落盘代次 → 校验和。校验和不一致视为迁移被改写，失败即关。
  func AppliedMap(rows []SchemaVer) map[int]string {
	  out := make(map[int]string, len(rows))
	  for _, r := range rows {
		   out[r.Version] = r.Checksum
	  }
	  return out
  }

   func CheckAppliedChecksum(applied map[int]string, m MigrationFile) error {
	   got, ok := applied[m.Version]
	   if !ok {
		  return nil
	   }
	  if got != "" && got != m.Checksum {
		   return fmt.Errorf("%w: checksum drift %s", ErrMigration, m.Name)
	   }
	  return nil
   }
