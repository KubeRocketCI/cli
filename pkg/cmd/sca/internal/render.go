package scainternal

import (
	"errors"
	"fmt"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
)

// HandleError is render.HandleError with portal.ErrUpstreamUnavailable
// prefixed by the Dependency-Track hint.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	if errors.Is(err, portal.ErrUpstreamUnavailable) {
		err = fmt.Errorf("upstream unavailable (dependency-track): %w", err)
	}

	return render.HandleError(ios, outputFormat, err)
}

// PageFooter formats the paging summary printed below a paginated sca table.
func PageFooter(noun string, total, pageIndex, pageSize int) string {
	totalPages := output.PageCount(total, pageSize)

	if total == 0 {
		return fmt.Sprintf("no %s (page-size %d)", pluralise(noun, 0), pageSize)
	}

	word := pluralise(noun, total)

	if pageIndex > totalPages {
		return fmt.Sprintf(
			"%d %s — page %d is beyond last page %d (page-size %d)",
			total, word, pageIndex, totalPages, pageSize,
		)
	}

	return fmt.Sprintf(
		"%d %s, page %d of %d (page-size %d)",
		total, word, pageIndex, totalPages, pageSize,
	)
}

func pluralise(noun string, n int) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// EmptyPlaceholder is the em-dash shown in sca table cells for a missing value.
const EmptyPlaceholder = "—"

// OrDash returns s when non-empty, otherwise EmptyPlaceholder. sca cells use
// this, not output.OrDash, which returns output.EmptyCell.
func OrDash(s string) string {
	if s == "" {
		return EmptyPlaceholder
	}
	return s
}

// BoolYesNo renders a bool as the lowercase "yes" / "no" used by every sca
// table column.
func BoolYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// FormatRisk renders a Dep-Track risk score, using EmptyPlaceholder for zero.
func FormatRisk(r float32) string {
	if r == 0 {
		return EmptyPlaceholder
	}
	return fmt.Sprintf("%.1f", r)
}

// FormatVulnCounts renders the C/H/M/L vulnerability counts column. The styled
// form is yellow when critical+high > 0.
func FormatVulnCounts(m *portal.SCAMetrics) output.Cell {
	if m == nil {
		return output.Text("0/0/0/0")
	}
	s := fmt.Sprintf("%d/%d/%d/%d", m.Critical, m.High, m.Medium, m.Low)
	if m.Critical+m.High > 0 {
		return output.Yellow(s)
	}
	return output.Text(s)
}
