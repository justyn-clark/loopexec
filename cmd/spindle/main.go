package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type smallStatus struct {
	Plan struct {
		NextActionable []string `json:"next_actionable"`
	} `json:"plan"`
}

func runSmall(args ...string) (string, error) {
	cmd := exec.Command("small", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show SMALL status and next actionable task",
		RunE: func(cmd *cobra.Command, _ []string) error {
			checkOutput, err := runSmall("check", "--strict")
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "HALT: small check --strict failed")
				if strings.TrimSpace(checkOutput) != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), strings.TrimSpace(checkOutput))
				}
				return errors.New("small check --strict failed")
			}

			statusOutput, err := runSmall("status", "--json")
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "ERROR: unable to read SMALL status")
				if strings.TrimSpace(statusOutput) != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), strings.TrimSpace(statusOutput))
				}
				return errors.New("small status failed")
			}

			var status smallStatus
			if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "ERROR: invalid SMALL status JSON")
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "SMALL strict check: PASS")

			if len(status.Plan.NextActionable) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "HALT: no actionable tasks")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Next actionable task: %s\n", status.Plan.NextActionable[0])
			return nil
		},
	}

	return cmd
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spindle",
		Short: "Spindle CLI",
	}

	cmd.AddCommand(newStatusCmd())
	return cmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
