package discovery

import (
	"strings"
	"testing"
)

func TestHostnameFromURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"https://foo.example.com/path", "foo.example.com"},
		{"http://bar.example.com", "bar.example.com"},
		{"foo.example.com", "foo.example.com"},
		{"https://baz.example.com:8080/api", "baz.example.com:8080"},
	}

	for _, c := range cases {
		got := HostnameFromURL(c.in)
		if got != c.want {
			t.Errorf("HostnameFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestShortDigestCell(t *testing.T) {
	t.Parallel()

	full := "sha256:abc1234567890def1234567890"
	if got := ShortDigestCell(&full); got != "sha256:abc12345" {
		t.Errorf("ShortDigestCell = %q, want sha256:abc12345", got)
	}

	if got := ShortDigestCell(nil); got != "-" {
		t.Errorf("ShortDigestCell(nil) = %q, want '-'", got)
	}

	empty := ""
	if got := ShortDigestCell(&empty); got != "-" {
		t.Errorf("ShortDigestCell(empty) = %q, want '-'", got)
	}
}

func TestIngressLines_Empty(t *testing.T) {
	t.Parallel()

	if got := IngressLines(nil, false); got != nil {
		t.Errorf("IngressLines(nil) = %v, want nil", got)
	}
}

func TestIngressLines_Multiple(t *testing.T) {
	t.Parallel()

	urls := []string{"https://foo.example.com", "https://bar.example.com/admin"}
	got := IngressLines(urls, false)

	if len(got) != 2 {
		t.Fatalf("IngressLines len = %d, want 2", len(got))
	}

	if got[0] != "foo.example.com" {
		t.Errorf("IngressLines[0] = %q, want foo.example.com", got[0])
	}

	if got[1] != "bar.example.com" {
		t.Errorf("IngressLines[1] = %q, want bar.example.com", got[1])
	}
}

func TestIngressLines_Truncates(t *testing.T) {
	t.Parallel()

	long := "https://" + strings.Repeat("a", 80) + ".example.com"
	got := IngressLines([]string{long}, false)

	if len(got) != 1 {
		t.Fatalf("IngressLines len = %d, want 1", len(got))
	}

	if len(got[0]) > MaxIngressHostLen {
		t.Errorf("hostname not truncated to MaxIngressHostLen: len=%d, host=%q", len(got[0]), got[0])
	}

	if !strings.HasSuffix(got[0], "...") {
		t.Errorf("expected truncated host to end with ..., got %q", got[0])
	}
}

func TestExpandIngressRows_NoIngress(t *testing.T) {
	t.Parallel()

	cells := []string{"foo", "healthy", "1.0"}
	rows := ExpandIngressRows(cells, nil)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	if rows[0][len(cells)] != "-" {
		t.Errorf("INGRESS cell = %q, want '-'", rows[0][len(cells)])
	}
}

func TestExpandIngressRows_Multiple(t *testing.T) {
	t.Parallel()

	cells := []string{"foo", "healthy", "1.0"}
	ingresses := []string{"a.example.com", "b.example.com", "c.example.com"}
	rows := ExpandIngressRows(cells, ingresses)

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// First row carries all data plus the first ingress
	for i, want := range cells {
		if rows[0][i] != want {
			t.Errorf("rows[0][%d] = %q, want %q", i, rows[0][i], want)
		}
	}

	// Subsequent rows blank out non-INGRESS columns.
	for r := range rows[1:] {
		for i := range cells {
			if rows[r+1][i] != "" {
				t.Errorf("rows[%d][%d] = %q, want '' (continuation row)", r+1, i, rows[r+1][i])
			}
		}
	}

	// Each row carries its own ingress in the last column.
	for r, row := range rows {
		if row[len(cells)] != ingresses[r] {
			t.Errorf("rows[%d] INGRESS = %q, want %q", r, row[len(cells)], ingresses[r])
		}
	}
}

func TestOptCell(t *testing.T) {
	t.Parallel()

	if got := OptCell(nil); got != "-" {
		t.Errorf("OptCell(nil) = %q", got)
	}

	v := "x"
	if got := OptCell(&v); got != "x" {
		t.Errorf("OptCell(&x) = %q", got)
	}

	empty := ""
	if got := OptCell(&empty); got != "-" {
		t.Errorf("OptCell(empty) = %q, want '-'", got)
	}
}

func TestColorForArgoStatus_Passthrough(t *testing.T) {
	t.Parallel()

	// Unknown / empty values pass through unstyled.
	if got := ColorForArgoStatus(""); got != "" {
		t.Errorf("ColorForArgoStatus('') = %q, want ''", got)
	}

	if got := ColorForArgoStatus("unknown"); got != "unknown" {
		t.Errorf("ColorForArgoStatus(unknown) = %q, want unknown", got)
	}
}
