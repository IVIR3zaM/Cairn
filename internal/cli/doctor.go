package cli

import (
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report detected languages and which standard tools are installed",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("doctor: not implemented")
			return nil
		},
	}
}
