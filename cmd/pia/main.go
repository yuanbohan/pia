// Command pia is the temporary Phase 1 one-shot coding-agent entrypoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yuanbohan/pia/internal/coding"
)

const (
	deepSeekAPIKeyEnv = "DEEPSEEK_API_KEY"
	tracePathEnv      = "PIA_TRACE_PATH"
)

type dependencies struct {
	lookupEnv  func(string) (string, bool)
	getwd      func() (string, error)
	run        func(context.Context, coding.RunInput) (coding.RunResult, error)
	buildTrace func(coding.RunResult, error) (coding.Trace, error)
	writeTrace func(string, coding.Trace) error
}

func main() {
	os.Exit(processMain(os.Args[1:], os.Stdout, os.Stderr, systemDependencies()))
}

func processMain(args []string, stdout, stderr io.Writer, deps dependencies) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runProcess(ctx, args, stdout, stderr, deps)
}

func runProcess(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) int {
	if err := execute(ctx, args, stdout, stderr, deps); err != nil {
		_, _ = fmt.Fprintf(stderr, "pia: %v\n", err)
		return 1
	}
	return 0
}

func execute(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("expected exactly one non-blank task argument")
	}
	task := args[0]

	apiKey, ok := deps.lookupEnv(deepSeekAPIKeyEnv)
	if !ok || strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("%s is required in the inherited process environment", deepSeekAPIKeyEnv)
	}
	workspacePath, err := deps.getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	tracePath := ""
	if configured, exists := deps.lookupEnv(tracePathEnv); exists && configured != "" {
		tracePath = resolveTracePath(workspacePath, configured)
	}

	result, runErr := deps.run(ctx, coding.RunInput{
		WorkspacePath: workspacePath,
		Task:          task,
		APIKey:        apiKey,
	})
	var traceErr error
	if tracePath != "" {
		// Trace creation intentionally happens after Run settlement and does not
		// reuse a canceled context. It preserves failure evidence but cannot roll
		// back Provider calls or tool mutations that already completed.
		trace, buildErr := deps.buildTrace(result, runErr)
		if buildErr != nil {
			traceErr = fmt.Errorf("build requested trace: %w", buildErr)
		} else if writeErr := deps.writeTrace(tracePath, trace); writeErr != nil {
			traceErr = fmt.Errorf("write requested trace: %w", writeErr)
		}
	}
	if combined := errors.Join(runErr, traceErr); combined != nil {
		return combined
	}
	for _, diagnostic := range result.SkillDiagnostics {
		if diagnostic.Path == "" {
			if _, err := fmt.Fprintf(stderr, "pia: warning: %q\n", diagnostic.Message); err != nil {
				return fmt.Errorf("write Skill diagnostic: %w", err)
			}
			continue
		}
		if _, err := fmt.Fprintf(stderr, "pia: warning: %q: %q\n", diagnostic.Path, diagnostic.Message); err != nil {
			return fmt.Errorf("write Skill diagnostic: %w", err)
		}
	}

	final := result.FinalText()
	if final == "" {
		return nil
	}
	if !strings.HasSuffix(final, "\n") {
		final += "\n"
	}
	if _, err := io.WriteString(stdout, final); err != nil {
		return fmt.Errorf("write final response: %w", err)
	}
	return nil
}

func resolveTracePath(workspacePath, configured string) string {
	if filepath.IsAbs(configured) {
		return configured
	}
	return filepath.Join(workspacePath, configured)
}

func systemDependencies() dependencies {
	return dependencies{
		lookupEnv:  os.LookupEnv,
		getwd:      os.Getwd,
		run:        coding.Run,
		buildTrace: coding.BuildTrace,
		writeTrace: writeTraceFile,
	}
}
