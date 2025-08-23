package mapper

import (
	"fmt"
	"os"
	"testing"
)

func TestGlyphOutlineMapper_MappingRune(t *testing.T) {
	readFontData, _ := os.ReadFile("read.ttf")
	miLantingFontData, _ := os.ReadFile("MI LANTING.ttf")
	mapper, err := NewGlyphOutlineMapper(readFontData, miLantingFontData)
	if err != nil {
		t.Fatal(err)
	}
	mapper.SetConcurrent(50)
	specialRune, standardRune, ok := mapper.MappingRune(0xE000)
	if !ok {
		t.Log("empty font data")
	}
	fmt.Printf("specialRune: %s => standardRune: %s\n", string(specialRune), string(standardRune))
}
