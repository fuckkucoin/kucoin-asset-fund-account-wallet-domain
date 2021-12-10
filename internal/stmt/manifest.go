package stmt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type File struct {
	Name string `json:"name"`
	Checksum string `json:"checksum"`
	Count    int    `json:"count"`
	Bytes int `json:"bytes"`
}

type Manifest struct {
	UserID string `json:"user_id,omitempty"`
	Files  []File `json:"files"`
	Root   string `json:"root"`
	Count  int    `json:"count"`
}

// BuildManifest 多文件 checksum 清单。Root 盖排序后的 name+checksum，缺一块就能查。
  func BuildManifest(userID string, files []Export) (Manifest, error) {
	   m := Manifest{UserID: userID, Files: make([]File, 0, len(files))}
	   for i, e := range files {
		  if err := VerifyExport(e); err != nil {
			   return Manifest{}, fmt.Errorf("file %d: %w", i, err)
		  }
		  name := e.UserID
		  if name == "" {
			  name = fmt.Sprintf("part-%d", i)
		  }
		  if e.Format != "" {
			   name = name + "." + e.Format
		  }
		   m.Files = append(m.Files, File{Name: name, Checksum: e.Checksum, Count: e.Count, Bytes: len(e.Body)})
	   }
	   sort.SliceStable(m.Files, func(i, j int) bool { return m.Files[i].Name < m.Files[j].Name })
	   h := sha256.New()
	  for _, f := range m.Files {
		   _, _ = h.Write([]byte(f.Name))
		   _, _ = h.Write([]byte{0})
		   _, _ = h.Write([]byte(f.Checksum))
		  _, _ = h.Write([]byte{0})
	  }
	   m.Root = hex.EncodeToString(h.Sum(nil))
	  m.Count = len(m.Files)
	   return m, nil
   }

   func VerifyManifest(m Manifest) error {
	   again, err := rebuildRoot(m.Files)
	  if err != nil {
		  return err
	  }
	  if again != m.Root {
		   return fmt.Errorf("manifest root mismatch")
	  }
	  return nil
  }

   func rebuildRoot(files []File) (string, error) {
	  cp := append([]File(nil), files...)
	  sort.SliceStable(cp, func(i, j int) bool { return cp[i].Name < cp[j].Name })
	   h := sha256.New()
	  for _, f := range cp {
		  if strings.TrimSpace(f.Checksum) == "" {
			  return "", fmt.Errorf("empty file checksum")
		   }
		  _, _ = h.Write([]byte(f.Name))
		   _, _ = h.Write([]byte{0})
		   _, _ = h.Write([]byte(f.Checksum))
		   _, _ = h.Write([]byte{0})
	   }
	  return hex.EncodeToString(h.Sum(nil)), nil
   }
