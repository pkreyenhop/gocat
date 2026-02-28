package editor

type gapBuffer struct {
	data     []rune
	gapStart int
	gapEnd   int
}

const minGap = 64

func newGapBufferFromRunes(rs []rune) gapBuffer {
	gap := minGap
	data := make([]rune, len(rs)+gap)
	copy(data, rs)
	return gapBuffer{data: data, gapStart: len(rs), gapEnd: len(rs) + gap}
}

func newGapBufferNoCopy(rs []rune) gapBuffer {
	if rs == nil {
		return gapBuffer{data: make([]rune, minGap), gapStart: 0, gapEnd: minGap}
	}
	return gapBuffer{data: rs, gapStart: len(rs), gapEnd: len(rs)}
}

func (g *gapBuffer) Len() int {
	return len(g.data) - (g.gapEnd - g.gapStart)
}

func (g *gapBuffer) Set(rs []rune) {
	*g = newGapBufferFromRunes(rs)
}

func (g *gapBuffer) ensureGap(n int) {
	if n <= g.gapEnd-g.gapStart {
		return
	}
	extra := n + minGap
	oldGap := g.gapEnd - g.gapStart
	newData := make([]rune, len(g.data)+extra)
	copy(newData, g.data[:g.gapStart])
	tailLen := len(g.data) - g.gapEnd
	newGapEnd := g.gapStart + oldGap + extra
	copy(newData[newGapEnd:newGapEnd+tailLen], g.data[g.gapEnd:])
	g.data = newData
	g.gapEnd = newGapEnd
}

func (g *gapBuffer) moveGap(pos int) {
	if pos < 0 {
		pos = 0
	}
	if pos > g.Len() {
		pos = g.Len()
	}
	if pos == g.gapStart {
		return
	}
	if pos < g.gapStart {
		delta := g.gapStart - pos
		copy(g.data[g.gapEnd-delta:g.gapEnd], g.data[pos:g.gapStart])
		g.gapStart -= delta
		g.gapEnd -= delta
		return
	}
	delta := pos - g.gapStart
	copy(g.data[g.gapStart:g.gapStart+delta], g.data[g.gapEnd:g.gapEnd+delta])
	g.gapStart += delta
	g.gapEnd += delta
}

func (g *gapBuffer) Insert(pos int, rs []rune) {
	if len(rs) == 0 {
		return
	}
	g.moveGap(pos)
	g.ensureGap(len(rs))
	copy(g.data[g.gapStart:g.gapStart+len(rs)], rs)
	g.gapStart += len(rs)
}

func (g *gapBuffer) Delete(start, end int) {
	if end <= start {
		return
	}
	if start < 0 {
		start = 0
	}
	if end > g.Len() {
		end = g.Len()
	}
	if end <= start {
		return
	}
	g.moveGap(start)
	g.gapEnd += end - start
}

func (g *gapBuffer) RuneAt(i int) (rune, bool) {
	if i < 0 || i >= g.Len() {
		return 0, false
	}
	if i < g.gapStart {
		return g.data[i], true
	}
	return g.data[i+(g.gapEnd-g.gapStart)], true
}

func (g *gapBuffer) Slice(a, b int) []rune {
	if a < 0 {
		a = 0
	}
	if b > g.Len() {
		b = g.Len()
	}
	if b <= a {
		return nil
	}
	out := make([]rune, b-a)
	for i := range out {
		r, _ := g.RuneAt(a + i)
		out[i] = r
	}
	return out
}

func (g *gapBuffer) Runes() []rune {
	if g.Len() == 0 {
		return nil
	}
	out := make([]rune, g.Len())
	copy(out, g.data[:g.gapStart])
	copy(out[g.gapStart:], g.data[g.gapEnd:])
	return out
}
