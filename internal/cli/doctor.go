package cli

import (
	"os"
	"os/exec"

	"github.com/IVIR3zaM/Cairn/internal/detect"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report detected languages and which standard tools are installed",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			res, err := detect.Detect(os.DirFS(wd), exec.LookPath)
			if err != nil {
				return err
			}
			detect.Render(cmd.OutOrStdout(), res)
			return nil
		},
	}
}
