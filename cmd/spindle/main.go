package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const smallTimeout = 10 * time.Second

type smallStatus struct {
	Plan struct {
		NextActionable []string `json:"next_actionable"`
	} `json:"plan"`
}

func runSmall(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), smallTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "small", args...)
	output, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return string(output), fmt.Errorf("small %s timed out", strings.Join(args, " "))
	}
	if err != nil {
		return string(output), fmt.Errorf("small %s failed: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "status",
		Short:        "Show SMALL status and next actionable task",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checkOutput, err := runSmall("check", "--strict")
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "HALT: small check --strict failed")
				fmt.Fprintf(cmd.ErrOrStderr(), "ERROR: %s\n", err.Error())
				if strings.TrimSpace(checkOutput) != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), strings.TrimSpace(checkOutput))
				}
				return err
			}

			statusOutput, err := runSmall("status", "--json")
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "ERROR: unable to read SMALL status")
				fmt.Fprintf(cmd.ErrOrStderr(), "ERROR: %s\n", err.Error())
				if strings.TrimSpace(statusOutput) != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), strings.TrimSpace(statusOutput))
				}
				return err
			}

			var status smallStatus
			if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "ERROR: invalid SMALL status JSON")
				fmt.Fprintf(cmd.ErrOrStderr(), "ERROR: %s\n", err.Error())
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
		Use:          "spindle",
		Short:        "Spindle CLI",
		SilenceUsage: true,
	}

	cmd.AddCommand(newStatusCmd())
	return cmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
