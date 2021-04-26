package adraw

import (
	"sync"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
)

var Font *draw.Font

type RefFont struct {
	lk  sync.Mutex
	Ref uint32
	F   *draw.Font
}

var RefFont1 RefFont

var RefFonts [2]*RefFont

var nfix int

func FindFont(fix, save, setfont bool, name string) *RefFont {
	var r *RefFont
	fixi := 0
	if fix {
		fixi = 1
		if nfix++; nfix > 1 {
			panic("fixi")
		}
	}
	if name == "" {
		name = FontNames[fixi]
		r = RefFonts[fixi]
	}
	if r == nil {
		for _, r = range FontCache {
			if r.F.Name == name {
				goto Found
			}
		}
		f, err := Display.OpenFont(name)
		if err != nil {
			alog.Printf("can't open font file %s: %v\n", name, err)
			return nil
		}
		r = new(RefFont)
		r.F = f
		FontCache = append(FontCache, r)
	}
Found:
	if save {
		util.Incref(&r.Ref)
		if RefFonts[fixi] != nil {
			CloseFont(RefFonts[fixi])
		}
		RefFonts[fixi] = r
		if name != FontNames[fixi] {
			FontNames[fixi] = name
		}
	}
	if setfont {
		RefFont1.F = r.F
		util.Incref(&r.Ref)
		CloseFont(RefFonts[0])
		Font = r.F
		RefFonts[0] = r
		util.Incref(&r.Ref)
		Init()
	}
	util.Incref(&r.Ref)
	return r
}

func CloseFont(r *RefFont) {
	if util.Decref(&r.Ref) == 0 {
		for i := range FontCache {
			if FontCache[i] == r {
				copy(FontCache[i:], FontCache[i+1:])
				FontCache = FontCache[:len(FontCache)-1]
				goto Found
			}
		}
		alog.Printf("internal error: can't find font in cache\n")
	Found:
		r.F.Free()
	}
}

var FontNames = []string{
	"/lib/font/bit/lucsans/euro.8.font",
	"/lib/font/bit/lucm/unicode.9.font",
}

var FontCache []*RefFont
