package quote_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfirmSourcesOmitHouseDebug(t *testing.T) {
	roots := []string{
		filepath.Join("..", "feeoracle"),
		filepath.Join("..", "fxmark"),
		filepath.Join("..", "swap"),
	}
	confirmFiles := []string{"confirm.go"}
	for _, root := range roots {
		for _, name := range confirmFiles {
			p := filepath.Join(root, name)
			body, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			s := string(body)
			if strings.Contains(s, "HouseDebugMid") || strings.Contains(s, "HouseDebugFee") || strings.Contains(s, "HouseDebugMark") || strings.Contains(s, "HouseDebugRate") {
				t.Fatalf("%s 引用了 house 调试价", p)
			}
			if strings.Contains(s, "HouseMid") {
				t.Fatalf("%s 引用 HouseMid", p)
			}
		}
	}
}
