package dproxy

type frame interface {
	// parentFrame returns parent frame.
	parentFrame() frame
	// frameLabel return label of frame.
	frameLabel() string
}

func fullAddress(f frame) string {
	x := 0
	for g := f; g != nil; g = g.parentFrame() {
		x += len(g.frameLabel())
	}
	if x == 0 {
		return "(root)"
	}
	b := make([]byte, x)
	for g := f; g != nil; g = g.parentFrame() {
		x -= len(g.frameLabel())
		copy(b[x:], g.frameLabel())
	}
	if b[0] == '.' {
		return string(b[1:])
	}
	return string(b)
}

type simpleFrame struct {
	parent frame
	label  string
}

var _ frame = (*simpleFrame)(nil)

func (f *simpleFrame) parentFrame() frame {
	return f.parent
}

func (f *simpleFrame) frameLabel() string {
	return f.label
}
