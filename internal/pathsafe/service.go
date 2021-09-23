// @FileName: service.go
// @Summary: 路径闸：拒 ..、空字节、命令拼接、绝对越权。只放过相对安全段。
// @Author: Hale
// @Created: 2021.09.22

package pathsafe

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

var (
	ErrPath   = fmt.Errorf("path refused")
	ErrInject = fmt.Errorf("path injection refused")
	ErrAbsolute  = fmt.Errorf("absolute path refused")
	ErrTraversal = fmt.Errorf("path traversal refused")
)

   func Check(raw string) (string, error) {
	  raw = strings.TrimSpace(raw)
	  if raw == "" {
		   return "", ErrPath
	   }
	  if strings.ContainsRune(raw, 0) {
		  return "", ErrInject
	   }
	   for _, r := range raw {
		   if unicode.IsControl(r) {
			   return "", ErrInject
		   }
	   }
	   for _, tok := range []string{";", "|", "`", "$(", "${", "&&", "||", ">", "<"} {
		   if strings.Contains(raw, tok) {
			  return "", ErrInject
		  }
	   }
	  if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "\\") || (len(raw) >= 2 && raw[1] == ':') {
		  return "", ErrAbsolute
	   }
	  clean := path.Clean(raw)
	  if strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") || clean == ".." {
		  return "", ErrTraversal
	  }
	  if strings.Contains(raw, "..") {
		  return "", ErrTraversal
	  }
	   if strings.Contains(raw, "://") {
		  return "", ErrInject
	  }
	  return clean, nil
  }

  func JoinRoot(root, child string) (string, error) {
	  c, err := Check(child)
	  if err != nil {
		   return "", err
	  }
	   root = path.Clean(strings.TrimSpace(root))
	   if root == "" || root == "." {
		   return "", ErrPath
	  }
	  out := path.Join(root, c)
	  if !strings.HasPrefix(out, root) {
		   return "", ErrTraversal
	   }
	  return out, nil
   }
// temporary guard removed after the production check
