package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"poster/internal/poster"

	"github.com/spf13/cobra"
)

type Mode int

const (
	urlMode Mode = iota
	serveMode
	deleteMode
)

func main() {
	var (
		outDir          string
		timeoutSource   time.Duration
		timeoutArtifact time.Duration
		notebookLMBin   string
		logLevel        string
		logFile         string
		delete          bool
		serve           bool
	)

	rootCmd := &cobra.Command{
		Use:   "poster [url]",
		Short: "Automates NotebookLM pipeline for YouTube URLs",
		Args: func(cmd *cobra.Command, args []string) error {
			if delete || serve {
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			var parsedLevel slog.Level
			if err := parsedLevel.UnmarshalText([]byte(logLevel)); err != nil {
				fmt.Fprintf(os.Stderr, "invalid log level %q: %v\n", logLevel, err)
				return err
			}

			text := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				AddSource: true,
				Level:     parsedLevel,
			})

			log := slog.New(text)
			if logFile != "" {
				f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to open log file %q: %v\n", logFile, err)
					return err
				}

				log = slog.New(slog.NewMultiHandler(
					text,
					slog.NewJSONHandler(f, &slog.HandlerOptions{
						AddSource: true,
						Level:     parsedLevel,
					}),
				))
			}

			slog.SetDefault(log)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := poster.ModeURL
			if delete {
				mode = poster.ModeDelete
			} else if serve {
				mode = poster.ModeServe
			}

			_main := func() error {
				_poster, err := poster.New(notebookLMBin, outDir, timeoutSource, timeoutArtifact)
				if err != nil {
					slog.Error("poster init failed", "error", err)
					return err
				}

				if err := loadDotEnv(".env"); err != nil {
					return err
				}

				token := toEnv(os.Getenv("TELEGRAM_BOT_TOKEN"))
				adminID := toEnv(os.Getenv("TELEGRAM_ADMIN_ID"))
				return _poster.Execute(args, mode, token, adminID)
			}

			if mode != poster.ModeServe {
				return _main()
			}

			for {
				if err := _main(); err != nil {
					slog.Error("serve loop failed, restarting", "error", err, "retry_in", "3s")
					time.Sleep(3 * time.Second)
					continue
				}
				return nil
			}
		},
	}

	flags := rootCmd.Flags()
	flags.StringVar(&outDir, "out", "./dist", "output directory for downloaded artifacts")
	flags.DurationVar(&timeoutSource, "timeout-source", 10*time.Minute, "source indexing timeout (e.g. 10m)")
	flags.DurationVar(&timeoutArtifact, "timeout-artifact", 15*time.Minute, "artifact generation timeout (e.g. 15m)")
	flags.StringVar(&notebookLMBin, "notebooklm-bin", "notebooklm", "path to notebooklm binary")
	flags.StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
	flags.StringVar(&logFile, "log-file", "", "optional path to JSON log file")
	flags.BoolVar(&delete, "delete-all", false, "delete all notebooks and exit")
	flags.BoolVar(&serve, "serve", false, "run telegram bot and process /yt commands")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func loadDotEnv(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}

	return nil
}

func toEnv(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, `"`+"'")
}
