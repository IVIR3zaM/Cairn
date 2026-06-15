package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/IVIR3zaM/Cairn/internal/detect"
	"github.com/IVIR3zaM/Cairn/internal/quality"
	"github.com/IVIR3zaM/Cairn/internal/report"
	"github.com/IVIR3zaM/Cairn/internal/runner"
	"github.com/spf13/cobra"
)

// errVerifyFailed makes verify exit non-zero. The compact summary already explains
// what failed, so the message itself stays silent (root sets SilenceErrors).
var errVerifyFailed = errors.New("verify failed")

func newVerifyCmd() *cobra.Command {
	var quiet, verbose bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run the configured quality gate (format, lint, test, …) for each language",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := config.LoadOrDefault("cairn.yaml")
			if err != nil {
				return err
			}
			res, err := detect.Detect(os.DirFS(wd), exec.LookPath)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			rep := report.New(out, report.Detect(out, quiet, verbose))
			rep.Start("cairn verify")

			run := runner.Exec{}
			var all []quality.Result
			for _, lang := range res.Languages {
				adapter, ok := quality.AdapterFor(lang.Name, run)
				if !ok {
					continue // no adapter registered for this language yet
				}
				if l, ok := cfg.Languages[lang.Name]; ok && !l.Enabled {
					continue // explicitly disabled in cairn.yaml
				}
				results := quality.Run(context.Background(), cfg.Verify, adapter,
					quality.LangUnit{Name: lang.Name, Dir: lang.Dir}, toolInfo(lang))
				all = append(all, results...)
			}

			steps := make([]report.Step, 0, len(all))
			for _, r := range all {
				s := report.Step{Name: r.Lang + " · " + r.Kind.String(), Status: toStatus(r.Status), Detail: r.Detail}
				rep.Step(s)
				steps = append(steps, s)
			}
			rep.Summary(steps)

			if quality.Failed(all) {
				return errVerifyFailed
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print the summary")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show full tool output")
	return cmd
}

// toolInfo turns a detected language's tool statuses into the gate's lookup map.
func toolInfo(l detect.Language) map[string]quality.ToolInfo {
	m := make(map[string]quality.ToolInfo, len(l.Tools))
	for _, t := range l.Tools {
		m[t.Tool.Name] = quality.ToolInfo{Installed: t.Installed, Hint: t.Tool.Hint}
	}
	return m
}

func toStatus(s quality.Status) report.Status {
	switch s {
	case quality.StatusPass:
		return report.Pass
	case quality.StatusFail:
		return report.Fail
	case quality.StatusWarn:
		return report.Warn
	default:
		return report.Skip
	}
}
