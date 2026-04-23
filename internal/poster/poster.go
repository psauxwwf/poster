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
	infographicStyle = `TASK:
Create one infographic image based on the source content.

VISUAL STYLE:
- Retro-futuristic, Fallout-inspired visual language
- 1950s poster look, worn paper texture
- Muted olive and amber palette
- Bold block headings and simple technical callouts
- Optimistic-but-cautionary mood
- Prefer a generic smiling retro mascot inspired by Vault Boy

TEXT RULES:
- Apply Fallout references only to visuals, not wording
- Text must match the source article and stay neutral
- Do not use franchise-like words such as "atomic", "vault", or similar
- Never mention "VAULT-TEC" in text or labels
- Keep all text highly readable and factually accurate`

	reportPrompt = `TASK:
Write a concise blog post in Markdown based on the source content.

LENGTH:
- If source video is under 10 minutes: target about 1024 characters
- If source video is 10+ minutes: no strict length limit

STYLE:
- Keep tone expressive and clear
- Emojis are allowed when appropriate
- Keep facts accurate and tied to the source
- Prefer dry, factual, and concise delivery over emotional wording
- Use italic emphasis at least once where it improves readability
- If the source contains command examples or other concrete examples, include as many of them as relevant
- Do not invent your own examples

ALLOWED MARKDOWN ONLY:
- italic
- underline
- strikethrough
- inline monospace code
- fenced code blocks
- language-tagged code blocks
- blockquotes
- collapsible quotes

RESTRICTIONS:
- Do not include links to any third-party resources
- Do not use raw HTML tags like <u>
- Do not use callout syntax like [!QUOTE]
- Do not ask for likes, subscriptions, comments, or other engagement actions`
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

func (p *Poster) run(chatID int64, url string) ([]byte, []byte, error) {
	if chatID != 0 {
		slog.Info("starting poster pipeline from telegram", "chat_id", chatID, "url", url)
	}

	result, err := p.notebooklm.Run(
		context.Background(),
		url,
		p.outDir,
		p.timeoutSource,
		p.timeoutArtifact,
		reportPrompt,
		infographicStyle,
	)
	if err != nil {
		if chatID != 0 {
			slog.Error("poster pipeline failed", "chat_id", chatID, "error", err)
		}
		return nil, nil, err
	}

	slog.Info(
		"outputs prepared",
		"report_bytes",
		len(result.Report),
		"image_bytes",
		len(result.Image),
	)

	if chatID != 0 {
		slog.Info("poster pipeline completed", "chat_id", chatID, "url", url)
	}

	return result.Image, result.Report, nil
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
		if _, _, err := p.run(0, url); err != nil {
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
	_adminID, err := strconv.ParseInt(strings.TrimSpace(adminID), 10, 64)
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
	if err := bot.Run(
		ctx,
		_adminID,
		p.run,
	); err != nil {
		return err
	}

	slog.Info("telegram bot stopped")
	return nil
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
