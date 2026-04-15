package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"poster/internal/poster"

	"github.com/spf13/cobra"
)

func main() {
	var (
		url             string
		outDir          string
		timeoutSource   time.Duration
		timeoutArtifact time.Duration
		notebookLMBin   string
		logLevel        string
		logFile         string
		deleteAll       bool
	)

	rootCmd := &cobra.Command{
		Use:   "poster <url>",
		Short: "Automates NotebookLM pipeline for YouTube URLs",
		Args: func(cmd *cobra.Command, args []string) error {
			if deleteAll {
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
			_poster, err := poster.New(outDir, timeoutSource, timeoutArtifact, notebookLMBin)
			if err != nil {
				slog.Error("poster init failed", "error", err)
				return err
			}

			if deleteAll {
				if err := _poster.DeleteAll(); err != nil {
					slog.Error("delete-all failed", "error", err)
					return err
				}
				return nil
			}

			url = args[0]
			slog.Info("starting poster pipeline", "url", url)
			if err := _poster.Run(url); err != nil {
				slog.Error("poster pipeline failed", "error", err)
				return err
			}
			slog.Info("poster pipeline completed")
			return nil
		},
	}

	flags := rootCmd.Flags()
	flags.StringVar(&outDir, "out", "./dist", "output directory for downloaded artifacts")
	flags.DurationVar(&timeoutSource, "timeout-source", 10*time.Minute, "source indexing timeout (e.g. 10m)")
	flags.DurationVar(&timeoutArtifact, "timeout-artifact", 15*time.Minute, "artifact generation timeout (e.g. 15m)")
	flags.StringVar(&notebookLMBin, "notebooklm-bin", "notebooklm", "path to notebooklm binary")
	flags.StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
	flags.StringVar(&logFile, "log-file", "", "optional path to JSON log file")
	flags.BoolVar(&deleteAll, "delete-all", false, "delete all notebooks and exit")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
