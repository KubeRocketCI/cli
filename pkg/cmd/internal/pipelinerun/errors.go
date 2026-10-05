package pipelinerun

import (
	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
)

// HandleError is cmdutil.HandleError with the envelope version of
// `pipelinerun start` and `project build`.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	return cmdutil.HandleError(ios, outputFormat, SchemaVersion, err)
}
