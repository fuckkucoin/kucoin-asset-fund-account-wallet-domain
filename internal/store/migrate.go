// 库表迁移。
// 作者: Span
// 日期: 2020.08.27
package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

  func ReadMigrations(dir string) ([]struct{ Version, SQL string }, error) {
	   files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	   if err != nil {
		   return nil, err
	  }
	  sort.Strings(files)
	   var out []struct {
		  Version,
		   SQL string
	   }
	   for _, f := range files {
		  b, err := os.ReadFile(f)
		   if err != nil {
			   return nil, err
		  }
		  ver := strings.TrimSuffix(filepath.Base(f), ".sql")
		   out = append(out, struct{ Version, SQL string }{ver, string(b)})
	  }
	  return out, nil
  }

  func SplitSQL(s string) []string {
	   raw := strings.Split(s, ";")
	  var out []string
	   for _, p := range raw {
		  p = strings.TrimSpace(p)
		  if p == "" || strings.HasPrefix(p, "--") && !strings.Contains(p, "\n") {
			   continue
		  }
		  out = append(out, p)
	  }
	  return out
   }

   func IgnorableMigrate(err error) bool {
	   if err == nil {
		  return true
	   }
	  m := strings.ToLower(err.Error())
	  return strings.Contains(m, "duplicate column") ||
		   strings.Contains(m, "duplicate key") ||
		   strings.Contains(m, "already exists")
  }
