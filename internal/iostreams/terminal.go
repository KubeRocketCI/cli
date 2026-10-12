package iostreams

import (
	"io"
	"sync"
	"unicode/utf8"

	"github.com/KubeRocketCI/cli/internal/termseq"
)

// Limits on a sequence under collection. A sequence past its limit is dropped.
const (
	maxCSILen = 64
	maxOSCLen = 2048
)

const (
	esc = 0x1b
	bel = 0x07
	can = 0x18
	sub = 0x1a
	del = 0x7f

	linkEnd = "\x1b]8;;\x1b\\"
)

type seqState uint8

const (
	ground   seqState = iota
	escSeen           // ESC
	escInter          // ESC and intermediates 0x20-0x2F
	csi               // ESC [
	osc               // ESC ]
	oscEsc            // ESC inside an OSC payload: ST when followed by \
	str               // DCS, SOS, PM, APC: dropped whole
	strEsc            // ESC inside a string
)

// terminalWriter is the sanitising stage of a process stream. Policy, applied
// to every write regardless of TTY state:
//
//   - Printable text, \n and \t pass. Invalid UTF-8 bytes become U+FFFD.
//   - \r\n becomes \n. A lone \r becomes \n.
//   - Every other C0 byte, DEL and the C1 runes U+0080-U+009F are dropped.
//   - SGR sequences pass when termseq.SGR accepts their parameters,
//     re-serialised canonically.
//   - OSC 8 passes when termseq.OSC8 accepts its payload, re-emitted with ST
//     and without params.
//   - Every other CSI, ESC, OSC, DCS, SOS, PM and APC sequence is dropped whole.
//   - In a CSI, ESC or OSC sequence a C0 byte, DEL or a byte of 0x80 or above
//     aborts the sequence: it is dropped and the byte is processed in ground
//     state. BEL ends an OSC, and ESC inside an OSC starts ST.
//   - A DCS, SOS, PM or APC payload ends at ST or BEL. Only \n, CAN and SUB
//     abort it.
//   - A sequence past its length limit is consumed to its terminator and
//     dropped whole.
//   - An open style or link is closed before \n.
//   - An incomplete sequence or rune is held until the next write and dropped
//     at exit.
//
// Write returns len(p) unless the next stage fails. Write is safe for
// concurrent use.
type terminalWriter struct {
	w  io.Writer
	mu sync.Mutex

	state seqState
	seq   []byte // CSI parameters or OSC payload under collection
	over  bool   // the sequence exceeded its limit and is dropped whole
	tail  []byte // incomplete UTF-8 rune carried to the next Write
	cr    bool   // \r waits for the next byte
	style bool   // an SGR other than reset is open
	link  bool   // an OSC 8 link is open

	out []byte
}

func newTerminalWriter(w io.Writer) *terminalWriter {
	return &terminalWriter{w: w}
}

func (t *terminalWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.out = t.out[:0]

	data := p
	if len(t.tail) > 0 {
		data = make([]byte, 0, len(t.tail)+len(p))
		data = append(data, t.tail...)
		data = append(data, p...)
		t.tail = t.tail[:0]
	}

	for len(data) > 0 {
		if t.state != ground {
			if t.collect(data[0]) {
				data = data[1:]
			}

			continue
		}

		b := data[0]

		if t.cr {
			t.cr = false

			if b != '\n' {
				t.newline()
			}
		}

		if b < utf8.RuneSelf {
			t.ascii(b)
			data = data[1:]

			continue
		}

		if !utf8.FullRune(data) {
			t.tail = append(t.tail[:0], data...)

			break
		}

		r, size := utf8.DecodeRune(data)
		data = data[size:]

		t.rune(r, size)
	}

	if _, err := t.w.Write(t.out); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (t *terminalWriter) ascii(b byte) {
	switch {
	case b == esc:
		t.state = escSeen
		t.seq = t.seq[:0]
	case b == '\n':
		t.newline()
	case b == '\r':
		t.cr = true
	case b == '\t' || (b >= 0x20 && b < del):
		t.out = append(t.out, b)
	}
}

func (t *terminalWriter) rune(r rune, size int) {
	switch {
	case r == utf8.RuneError && size == 1:
		t.out = utf8.AppendRune(t.out, utf8.RuneError)
	case r >= 0x80 && r <= 0x9f:
	default:
		t.out = utf8.AppendRune(t.out, r)
	}
}

func (t *terminalWriter) newline() {
	if t.link {
		t.out = append(t.out, linkEnd...)
		t.link = false
	}

	if t.style {
		t.out = append(t.out, termseq.Reset...)
		t.style = false
	}

	t.out = append(t.out, '\n')
}

func (t *terminalWriter) reset() {
	t.state = ground
	t.seq = t.seq[:0]
	t.over = false
}

// collect feeds one byte to the sequence under collection and reports whether
// the byte was consumed. An unconsumed byte aborted the sequence and is
// processed again in the new state.
func (t *terminalWriter) collect(b byte) bool {
	switch t.state {
	case escSeen, escInter:
		return t.collectEsc(b)
	case csi:
		return t.collectCSI(b)
	case osc:
		return t.collectOSC(b)
	case oscEsc:
		return t.collectTerminator(b, t.finishOSC)
	case str:
		return t.collectString(b)
	case strEsc:
		return t.collectTerminator(b, t.reset)
	case ground:
	}

	return true
}

// collectEsc handles the byte after ESC or after its intermediates. Only the
// byte right after ESC can introduce a CSI, OSC or string.
func (t *terminalWriter) collectEsc(b byte) bool {
	if t.state == escSeen {
		switch b {
		case '[':
			t.state = csi

			return true
		case ']':
			t.state = osc

			return true
		case 'P', 'X', '^', '_':
			t.state = str

			return true
		}
	}

	switch {
	case b >= 0x20 && b <= 0x2f:
		t.state = escInter
	case b >= 0x30 && b <= 0x7e:
		t.reset()
	default:
		t.reset()

		return false
	}

	return true
}

func (t *terminalWriter) collectCSI(b byte) bool {
	switch {
	case b >= 0x40 && b <= 0x7e:
		t.finishCSI(b)
	case b >= 0x20 && b <= 0x3f:
		t.store(b, maxCSILen)
	default:
		t.reset()

		return false
	}

	return true
}

func (t *terminalWriter) collectOSC(b byte) bool {
	switch b {
	case bel:
		t.finishOSC()
	case esc:
		t.state = oscEsc
	default:
		if b < 0x20 || b > 0x7e {
			t.reset()

			return false
		}

		t.store(b, maxOSCLen)
	}

	return true
}

// collectString consumes a DCS, SOS, PM or APC payload, which is never kept.
func (t *terminalWriter) collectString(b byte) bool {
	switch b {
	case esc:
		t.state = strEsc
	case bel:
		t.reset()
	case '\n', can, sub:
		t.reset()

		return false
	}

	return true
}

// collectTerminator handles the byte after an ESC inside a string: a backslash
// completes ST and calls finish. Anything else aborts the string, and the ESC
// starts a new sequence with b.
func (t *terminalWriter) collectTerminator(b byte, finish func()) bool {
	if b == '\\' {
		finish()

		return true
	}

	t.reset()
	t.state = escSeen

	return false
}

// store keeps b for the sequence under collection, or marks the sequence
// dropped once it exceeds limit.
func (t *terminalWriter) store(b byte, limit int) {
	if len(t.seq) < limit {
		t.seq = append(t.seq, b)

		return
	}

	t.over = true
}

func (t *terminalWriter) finishCSI(final byte) {
	params, over := string(t.seq), t.over
	t.reset()

	if final != 'm' || over {
		return
	}

	canon, open, ok := termseq.SGR(params)
	if !ok {
		return
	}

	t.out = append(t.out, canon...)
	t.style = open
}

func (t *terminalWriter) finishOSC() {
	payload, over := string(t.seq), t.over
	t.reset()

	if over {
		return
	}

	target, ok := termseq.OSC8(payload)
	if !ok {
		return
	}

	if target == "" {
		if t.link {
			t.out = append(t.out, linkEnd...)
			t.link = false
		}

		return
	}

	t.out = append(t.out, "\x1b]8;;"...)
	t.out = append(t.out, target...)
	t.out = append(t.out, "\x1b\\"...)
	t.link = true
}
