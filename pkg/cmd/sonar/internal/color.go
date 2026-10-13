package sonarinternal

import (
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// GateStatusColor colors a SonarQube quality-gate status. An unknown value is
// made printable on one row and left unstyled.
func GateStatusColor(status portal.QualityGateStatus) output.Cell {
	s := string(status)

	switch status {
	case portal.QualityGateOK:
		return output.Green(s)
	case portal.QualityGateWarn:
		return output.Yellow(s)
	case portal.QualityGateError:
		return output.Red(s)
	case portal.QualityGateNone:
		return output.Gray(s)
	default:
		return output.Text(s)
	}
}
