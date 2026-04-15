package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, binary string, args ...string) (string, string, error) {
	command := binary + " " + strings.Join(args, " ")

	cmd := exec.CommandContext(ctx, binary, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	stdoutText := strings.TrimSpace(stdout.String())
	stderrText := strings.TrimSpace(stderr.String())

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return stdoutText, stderrText, fmt.Errorf("command `%s` timed out", command)
		}

		reason := stderrText
		if reason == "" {
			reason = stdoutText
		}
		if reason == "" {
			reason = err.Error()
		}

		return stdoutText, stderrText, fmt.Errorf("command `%s` failed: %s", command, reason)
	}

	return stdoutText, stderrText, nil
}
