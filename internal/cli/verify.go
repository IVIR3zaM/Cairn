package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/IVIR3zaM/Cairn/internal/detect"
	"github.com/IVIR3zaM/Cairn/internal/quality"
	golang "github.com/IVIR3zaM/Cairn/internal/quality/go"
	"github.com/IVIR3zaM/Cairn/internal/quality/rust"
	"github.com/IVIR3zaM/Cairn/internal/report"
	"github.com/IVIR3zaM/Cairn/internal/runner"
	"github.com/spf13/cobra"
)

// adapters maps a detected language to its quality adapter. Wiring a new language's
// adapter (iteration 5) is a one-line entry here.
var adapters = map[string]func(runner.ToolRunner) quality.Adapter{
	"go":   func(r runner.ToolRunner) quality.Adapter { return golang.New(r) },
	"rust": func(r runner.ToolRunner) quality.Adapter { return rust.New(r) },
}

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
				ctor := adapters[lang.Name]
				if ctor == nil {
					continue // no adapter for this language yet
				}
				if l, ok := cfg.Languages[lang.Name]; ok && !l.Enabled {
					continue // explicitly disabled in cairn.yaml
				}
				results := quality.Run(context.Background(), cfg.Verify, ctor(run),
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
