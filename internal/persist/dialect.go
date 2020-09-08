// 功能：persist 模块 dialect。
// SQL 方言。业务仓储只写一份占位符 SQL，落地前按 MySQL / Postgres 改写。

// 作者：TianpengLan
// 日期：2020.08.31

package persist

import (
	"strconv"
	"strings"
)

type Dialect int

const (
	MySQL Dialect = iota
	Postgres
)

   func (d Dialect) String() string {
	  if d == Postgres {
		  return "postgres"
	   }
	   return "mysql"
  }

   func ParseDialect(s string) Dialect {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	  case "postgres", "postgresql", "pg":
		  return Postgres
	   default:
		  return MySQL
	  }
   }

// Rewrite 把 ? 占位和 MySQL 类型改成目标方言。不得改写字符串常量里的问号。

   func Rewrite(sql string, d Dialect) string {
	   if d != Postgres {
		  return sql
	  }
	   out := strings.ReplaceAll(sql, "ALTER TABLE aa_deposits DROP PRIMARY KEY", "ALTER TABLE aa_deposits DROP CONSTRAINT IF EXISTS aa_deposits_pkey")
	  out = strings.ReplaceAll(out, "ALTER TABLE chain_tx_history DROP INDEX uk_chain_tx", "ALTER TABLE chain_tx_history DROP CONSTRAINT IF EXISTS uk_chain_tx")
	  out = strings.ReplaceAll(out, "DATETIME(3)", "TIMESTAMPTZ")
	  out = strings.ReplaceAll(out, "TINYINT", "SMALLINT")
	  out = strings.ReplaceAll(out, "BLOB", "BYTEA")
	  out = strings.ReplaceAll(out, "CURRENT_TIMESTAMP(3)", "CURRENT_TIMESTAMP")
	   out = strings.ReplaceAll(out, "BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY", "BIGSERIAL PRIMARY KEY")
	   out = strings.ReplaceAll(out, "INT NOT NULL AUTO_INCREMENT PRIMARY KEY", "SERIAL PRIMARY KEY")
	   out = rewriteInsertIgnore(out)
	  out = rewriteUniqueKey(out)
	  return rewritePlaceholders(out)
   }

   func rewriteInsertIgnore(sql string) string {
	  if !strings.Contains(sql, "INSERT IGNORE INTO") {
		  return sql
	   }
	  sql = strings.ReplaceAll(sql, "INSERT IGNORE INTO", "INSERT INTO")
	  if strings.Contains(strings.ToUpper(sql), "ON CONFLICT") {
		  return sql
	   }
	  sql = strings.TrimSpace(sql)
	  sql = strings.TrimSuffix(sql, ";")
	  return sql + " ON CONFLICT DO NOTHING"
  }

   func rewritePlaceholders(sql string) string {
	  var b strings.Builder
	   n := 0
	  inStr := false
	   for i := 0; i < len(sql); i++ {
		  c := sql[i]
		  if c == '\'' {
			  b.WriteByte(c)
			  if inStr && i+1 < len(sql) && sql[i+1] == '\'' {
				   b.WriteByte(sql[i+1])
				  i++
				  continue
			  }
			   inStr = !inStr
			   continue
		  }
		   if c == '?' && !inStr {
			   n++
			  b.WriteByte('$')
			  b.WriteString(strconv.Itoa(n))
			   continue
		   }
		   b.WriteByte(c)
	  }
	  return b.String()
  }

  func rewriteUniqueKey(sql string) string {
	  // UNIQUE KEY name (cols) → UNIQUE (cols)
	   for {
		   i := strings.Index(sql, "UNIQUE KEY ")
		  if i < 0 {
			   break
		  }
		   rest := sql[i+len("UNIQUE KEY "):]
		   j := strings.Index(rest, "(")
		  if j < 0 {
			   break
		  }
		   sql = sql[:i] + "UNIQUE " + rest[j:]
	  }
	  // KEY name (cols) → 留给独立 CREATE INDEX；DDL 里先删掉以免 Postgres 拒。
	  for {
		   i := indexBareKey(sql)
		   if i < 0 {
			   break
		  }
		  rest := sql[i:]
		  end := strings.Index(rest, ")")
		  if end < 0 {
			   break
		   }
		   // 吃掉前面的逗号
		  start := i
		  if start > 0 && sql[start-1] == ',' {
			   start--
		   }
		  sql = sql[:start] + rest[end+1:]
	  }
	   return sql
  }

   func indexBareKey(sql string) int {
	   for i := 0; i < len(sql)-4; i++ {
		  if !strings.HasPrefix(sql[i:], "KEY ") {
			   continue
		  }
		  if i >= 7 && strings.EqualFold(sql[i-7:i], "UNIQUE ") {
			  continue
		   }
		  if i >= 8 && strings.EqualFold(sql[i-8:i], "PRIMARY ") {
			   continue
		  }
		  return i
	   }
	  return -1
  }
