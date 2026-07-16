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

	"github.com/spf13/cobra"

	"poster/pkg/notebooklm"
	"poster/pkg/tg"
)

const (
	infographicStyle = `TASK:
Create one infographic image based on the source content.

VISUAL STYLE:
- Simple hand-drawn sketch-note visual language
- Clean light background with high contrast
- Large readable headings and short labels
- Minimal decorative details, focused on clarity
- Use simple icons, arrows, boxes, and small diagrams
- Mobile-first portrait composition with readable text on small screens
- Friendly explanatory drawing style, easy to understand at a glance

TEXT RULES:
- Text must match the source article and stay neutral
- Keep all text highly readable and factually accurate
- Prefer short labels over long sentences
- Do not add facts that are not present in the source`

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
- Do not use Markdown tables
- If table-like data is needed, convert it into plain bullet lists
- Do not ask for likes, subscriptions, comments, or other engagement actions`

	audioStyle = `TASK:
Create one short audio script from the source content.

DELIVERY:
- Very fast pace
- Dry, factual tone
- No filler words, no small talk, no motivational phrases
- No rhetorical questions
- Focus on concrete points only

STRUCTURE:
- Start with a one-line topic statement
- Then give dense fact blocks in logical order
- Keep transitions minimal and functional
- End with a brief factual wrap-up

RULES:
- Do not add information that is not in the source
- Keep wording neutral and precise
- Prefer numbers, names, commands, and key specifics when available
- Avoid jokes, storytelling, and emotional language`
)

type Poster struct {
	notebooklm *notebooklm.NotebookLM
}

type Mode int

const (
	ModeURL Mode = iota
	ModeServe
	ModeDelete
)

func New(_notebookLMBinary string, outDir string, timeoutSource, timeoutArtifact time.Duration) (*Poster, error) {
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		outDir = "./dist/notebooklm"
	}

	if timeoutSource <= 0 {
		timeoutSource = 10 * time.Minute
	}

	if timeoutArtifact <= 0 {
		timeoutArtifact = 15 * time.Minute
	}

	nlm, err := notebooklm.New(_notebookLMBinary, outDir, timeoutSource, timeoutArtifact)
	if err != nil {
		return nil, err
	}

	return &Poster{
		notebooklm: nlm,
	}, nil
}

func (p *Poster) run(chatID int64, urls []string, outputs notebooklm.Outputs) (notebooklm.Out, error) {
	if chatID != 0 {
		slog.Info("starting poster pipeline from telegram", "chat_id", chatID, "urls", urls)
	}

	r, err := p.notebooklm.Run(
		context.Background(),
		urls,
		outputs,
		reportPrompt,
		infographicStyle,
		audioStyle,
	)
	if err != nil {
		if chatID != 0 {
			slog.Error("poster pipeline failed", "chat_id", chatID, "urls", urls, "error", err)
		}
		return notebooklm.Out{}, err
	}

	slog.Info(
		"outputs prepared",
		"report_bytes",
		len(r.Report.Data),
		"image_bytes",
		len(r.Image.Data),
	)

	if chatID != 0 {
		slog.Info("poster pipeline completed", "chat_id", chatID, "urls", urls)
	}

	return r, nil
}

func (p *Poster) Execute(cmd *cobra.Command, args []string, mode Mode) error {
	switch mode {
	case ModeDelete:
		ids, err := p.DeleteAll()
		if err != nil {
			slog.Error("delete-all failed", "error", err)
			return err
		}
		if cmd != nil {
			for _, id := range ids {
				cmd.Println("deleted notebook:", id)
			}
			cmd.Println("delete completed")
		}
		return nil
	case ModeURL:
		slog.Info("starting poster pipeline", "args", args)
		res, err := p.run(0, args, notebooklm.FullOutputs())
		if err != nil {
			slog.Error("poster pipeline failed", "error", err)
			return err
		}
		slog.Info(
			"poster artifacts saved",
			"image_path",
			res.Image.Path,
			"report_path",
			res.Report.Path,
		)
		slog.Info("poster pipeline completed")
		if cmd != nil {
			cmd.Println("image:", res.Image.Path)
			cmd.Println("report:", res.Report.Path)
			cmd.Println("audio:", res.Audio.Path)
		}
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

func (p *Poster) DeleteAll() ([]string, error) {
	ctx := context.Background()

	ids, err := p.notebooklm.ListNotebookIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list notebooks: %w", err)
	}
	if len(ids) == 0 {
		slog.Info("no notebooks to delete")
		return nil, nil
	}

	deleted := make([]string, 0, len(ids))
	slog.Info("deleting notebooks", "count", len(ids))
	for _, id := range ids {
		if err := p.notebooklm.DeleteNotebook(ctx, id); err != nil {
			return deleted, fmt.Errorf("failed to delete notebook %s: %w", id, err)
		}
		slog.Info("deleted notebook", "notebook_id", id)
		deleted = append(deleted, id)
	}

	slog.Info("delete-all completed", "deleted", len(ids))
	return deleted, nil
}
