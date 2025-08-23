package main

import (
	"fmt"
	"sync"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type GlyphOutlineMapper struct {
	specialFont  *truetype.Font
	standardFont *truetype.Font
	concurrent   int
	wg           *sync.WaitGroup
	sem          chan struct{}
}

func NewGlyphOutlineMapper(specialFontData, standardFontData []byte) (*GlyphOutlineMapper, error) {
	mapper := GlyphOutlineMapper{
		concurrent: 10,
		wg:         &sync.WaitGroup{},
		sem:        make(chan struct{}, 10),
	}

	specialFont, err := truetype.Parse(specialFontData)
	if err != nil {
		return nil, fmt.Errorf("parse special font failed: %w", err)
	}
	mapper.specialFont = specialFont

	standardFont, err := truetype.Parse(standardFontData)
	if err != nil {
		return nil, fmt.Errorf("parse standard font failed: %w", err)
	}
	mapper.standardFont = standardFont
	return &mapper, nil
}

func (g *GlyphOutlineMapper) SetConcurrent(concurrent int) {
	g.concurrent = concurrent
	g.sem = make(chan struct{}, concurrent)
}

func (g *GlyphOutlineMapper) GlyphOutlineEqual(specialUnicode, standardUnicode rune) (bool, error) {
	// 获取字符在字体中的索引
	index1 := g.specialFont.Index(specialUnicode)
	index2 := g.standardFont.Index(standardUnicode)

	if index1 == 0 || index2 == 0 {
		return false, nil // 字符不存在
	}

	// 获取字形轮廓数据
	var buf1, buf2 truetype.GlyphBuf
	err := buf1.Load(g.specialFont, fixed.I(1000), index1, font.HintingNone)
	if err != nil {
		return false, fmt.Errorf("加载专有字体失败: %w", err)
	}
	err = buf2.Load(g.standardFont, fixed.I(1000), index2, font.HintingNone)
	if err != nil {
		return false, fmt.Errorf("加载标准字体失败: %w", err)
	}

	// 实际比较轮廓数据
	return g.compareGlyphOutlines(&buf1, &buf2), nil
}

// compareGlyphOutlines 比较两个字形的轮廓数据
func (g *GlyphOutlineMapper) compareGlyphOutlines(buf1, buf2 *truetype.GlyphBuf) bool {
	// 1. 比较轮廓数量
	if len(buf1.Ends) != len(buf2.Ends) {
		return false
	}

	// 2. 比较每个轮廓的端点
	for i := range buf1.Ends {
		if buf1.Ends[i] != buf2.Ends[i] {
			return false
		}
	}

	// 3. 比较轮廓点的数量
	if len(buf1.Points) != len(buf2.Points) {
		return false
	}

	// 4. 比较每个轮廓点的坐标（允许小的浮点误差）
	tolerance := fixed.Int26_6(10) // 允许的误差范围
	for i := range buf1.Points {
		dx := buf1.Points[i].X - buf2.Points[i].X
		dy := buf1.Points[i].Y - buf2.Points[i].Y

		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}

		if dx > tolerance || dy > tolerance {
			return false
		}
	}

	return true
}

func (g *GlyphOutlineMapper) Mapping(start, end rune) map[rune]rune {
	results := &sync.Map{}
	for i := start; i <= end; i++ {
		g.wg.Add(1)
		g.sem <- struct{}{}
		go func(i rune) {
			defer g.wg.Done()
			defer func() { <-g.sem }()

			specialRune, standardRune, ok := g.MappingRune(i)
			if ok {
				results.Store(specialRune, standardRune)
			}
		}(i)
	}
	g.wg.Wait()
	close(g.sem)
	resultsMap := map[rune]rune{}
	results.Range(func(key, value any) bool {
		resultsMap[key.(rune)] = value.(rune)
		return true
	})
	return resultsMap
}

func (g *GlyphOutlineMapper) MappingRune(unicode rune) (specialRune, standardRune rune, ok bool) {
	if ok, _ = g.hasGlyph(g.specialFont, unicode); !ok {
		return
	}
	for j := 0x4e00; j <= 0x9fff; j++ {
		if ok, _ = g.hasGlyph(g.standardFont, rune(j)); !ok {
			continue
		}
		if ok, _ = g.GlyphOutlineEqual(rune(unicode), rune(j)); ok {
			specialRune = rune(unicode)
			standardRune = rune(j)
			return
		}
	}
	return
}

func (g *GlyphOutlineMapper) hasGlyph(font *truetype.Font, char rune) (bool, string) {
	if font == nil {
		return false, "字体未加载"
	}

	// 方法1：检查字体索引
	index := font.Index(char)
	if index == 0 && char != 0 {
		return false, "字符索引为0（字符不存在）"
	}

	// 方法2：检查字形边界和advance
	face := truetype.NewFace(font, &truetype.Options{Size: 12})
	defer face.Close()

	bounds, advance, ok := face.GlyphBounds(char)
	if !ok {
		return false, "无法获取字形边界"
	}

	// 方法3：检查是否有实际的可视字形
	if bounds.Empty() && advance == 0 {
		return false, "空边界且无宽度"
	}

	// 方法4：对于私有使用区域的特殊检查
	if char >= 0xE000 && char <= 0xF8FF {
		// 私有使用区域，即使bounds为空也可能有字形
		if advance > 0 {
			return true, "私有区域有宽度"
		}
		if !bounds.Empty() {
			return true, "私有区域有边界"
		}
		if index > 0 {
			return true, "私有区域有索引"
		}
		return false, "私有区域无数据"
	}

	// 一般情况下，有索引就认为存在
	if index > 0 {
		return true, "有效索引"
	}

	return false, "未知原因"
}
