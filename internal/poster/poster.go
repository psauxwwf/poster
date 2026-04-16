package poster

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"poster/pkg/notebooklm"
	"poster/pkg/tg"
)

const (
	infographicStyle = "Design a retro-futuristic Fallout-inspired infographic: 1950s atomic-age poster aesthetic, worn paper texture, muted olive and amber palette, bold block headings, simple technical callouts, and optimistic-but-cautionary tone. Prefer using a generic smiling retro mascot inspired by Vault Boy in the imagery. Do not mention VAULT-TEC anywhere in the text or labels. Keep all text highly readable and preserve factual accuracy from the source."
	reportPrompt     = "Write a concise blog post in Markdown. If the source video is short (under 10 minutes), keep the post around 1024 characters; if it is longer, no strict length limit. Use expressive formatting and emojis where appropriate. Use only the following Markdown formatting: italic, underline, strikethrough, inline monospace code, fenced code blocks, language-tagged code blocks, blockquotes, and collapsible quotes. Do not include links to any third-party resources. Keep facts accurate and tied to the source."
)

type Poster struct {
	notebooklm *notebooklm.NotebookLM
	outDir     string

	timeoutSource   time.Duration
	timeoutArtifact time.Duration
}

type Mode int

const (
	ModeURL Mode = iota
	ModeServe
	ModeDelete
)

func New(_outDir string, _timeoutSource, _timeoutArtifact time.Duration, _notebookLMBinary string) (*Poster, error) {
	_outDir = strings.TrimSpace(_outDir)
	if _outDir == "" {
		_outDir = "./dist/notebooklm"
	}

	if _timeoutSource <= 0 {
		_timeoutSource = 10 * time.Minute
	}

	if _timeoutArtifact <= 0 {
		_timeoutArtifact = 15 * time.Minute
	}

	nlm, err := notebooklm.New(_notebookLMBinary)
	if err != nil {
		return nil, err
	}

	return &Poster{
		notebooklm:      nlm,
		outDir:          _outDir,
		timeoutSource:   _timeoutSource,
		timeoutArtifact: _timeoutArtifact,
	}, nil
}

func (p *Poster) ytRun(url string) (notebooklm.YouTubePipelineOutput, error) {
	result, err := p.notebooklm.RunYouTubePipeline(
		context.Background(),
		url,
		p.outDir,
		p.timeoutSource,
		p.timeoutArtifact,
		reportPrompt,
		infographicStyle,
	)
	if err != nil {
		return notebooklm.YouTubePipelineOutput{}, err
	}

	slog.Info(
		"outputs prepared",
		"report_bytes",
		len(result.Report),
		"image_bytes",
		len(result.Image),
	)

	return result, nil
}

func (p *Poster) Execute(args []string, mode Mode, token string, adminID string) error {
	switch mode {
	case ModeDelete:
		if err := p.DeleteAll(); err != nil {
			slog.Error("delete-all failed", "error", err)
			return err
		}
		return nil
	case ModeServe:
		if err := p.Serve(token, adminID); err != nil {
			slog.Error("telegram bot stopped with error", "error", err)
			return err
		}
		return nil
	case ModeURL:
		url := args[0]
		slog.Info("starting poster pipeline", "url", url)
		if _, err := p.ytRun(url); err != nil {
			slog.Error("poster pipeline failed", "error", err)
			return err
		}
		slog.Info("poster pipeline completed")
		return nil
	default:
		return fmt.Errorf("unknown mode: %d", mode)
	}
}

func (p *Poster) Serve(token string, adminID string) error {
	parsedAdminID, err := strconv.ParseInt(strings.TrimSpace(adminID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid TELEGRAM_ADMIN_ID: %w", err)
	}

	bot, err := tg.New(token)
	if err != nil {
		return fmt.Errorf("telegram bot init failed: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("telegram bot started", "mode", "serve")
	if err := bot.Run(ctx, parsedAdminID, p.ytHandler()); err != nil {
		return err
	}

	slog.Info("telegram bot stopped")
	return nil
}

func (p *Poster) ytHandler() func(chatID int64, ytURL string) ([]byte, []byte, error) {
	return func(chatID int64, ytURL string) ([]byte, []byte, error) {
		slog.Info("starting poster pipeline from telegram", "chat_id", chatID, "url", ytURL)
		result, err := p.ytRun(ytURL)
		if err != nil {
			slog.Error("poster pipeline failed", "chat_id", chatID, "error", err)
			return nil, nil, err
		}

		slog.Info("poster pipeline completed", "chat_id", chatID, "url", ytURL)
		return result.Image, result.Report, nil
	}
}

func (p *Poster) DeleteAll() error {
	ctx := context.Background()

	ids, err := p.notebooklm.ListNotebookIDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to list notebooks: %w", err)
	}
	if len(ids) == 0 {
		slog.Info("no notebooks to delete")
		return nil
	}

	slog.Info("deleting notebooks", "count", len(ids))
	for _, id := range ids {
		if err := p.notebooklm.DeleteNotebook(ctx, id); err != nil {
			return fmt.Errorf("failed to delete notebook %s: %w", id, err)
		}
		slog.Info("deleted notebook", "notebook_id", id)
	}

	slog.Info("delete-all completed", "deleted", len(ids))
	return nil
}
