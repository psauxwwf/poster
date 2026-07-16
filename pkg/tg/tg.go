package tg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"

	"poster/pkg/notebooklm"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Tg struct {
	bot      *tgbotapi.BotAPI
	reurl    *regexp.Regexp
	updateTO int
}

var botCommands = []tgbotapi.BotCommand{
	{Command: "start", Description: "show available commands"},
	{Command: "post", Description: "send image and blog summary for sources"},
	{Command: "full", Description: "send image, blog summary, and audio for sources"},
	{Command: "summary", Description: "send only blog summary for sources"},
	{Command: "image", Description: "send only infographic image for sources"},
}

type commandSpec struct {
	Usage   string
	Outputs notebooklm.Outputs
}

var commandSpecs = map[string]commandSpec{
	"post": {
		Usage:   "/post <source-url>",
		Outputs: notebooklm.Outputs{Report: true, Image: true},
	},
	"full": {
		Usage:   "/full <source-url>",
		Outputs: notebooklm.FullOutputs(),
	},
	"summary": {
		Usage:   "/summary <source-url>",
		Outputs: notebooklm.Outputs{Report: true},
	},
	"image": {
		Usage:   "/image <source-url>",
		Outputs: notebooklm.Outputs{Image: true},
	},
}

const telegramPhotoCaptionLimit = 1024

func New(token string) (*Tg, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("empty telegram token")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}

	return &Tg{
		bot:      bot,
		reurl:    regexp.MustCompile(`https?://\S+`),
		updateTO: 30,
	}, nil
}

func (t *Tg) RegisterCommands() error {
	_, err := t.bot.Request(tgbotapi.NewSetMyCommands(botCommands...))
	if err != nil {
		return fmt.Errorf("set telegram commands: %w", err)
	}

	return nil
}

func (t *Tg) Run(ctx context.Context, adminID int64, f func(int64, []string, notebooklm.Outputs) (notebooklm.Out, error)) error {
	if f == nil {
		return fmt.Errorf("handler is nil")
	}
	if adminID == 0 {
		return fmt.Errorf("adminID is required")
	}

	if err := t.RegisterCommands(); err != nil {
		return err
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = t.updateTO

	updates := t.bot.GetUpdatesChan(u)
	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if update.Message == nil || !update.Message.IsCommand() {
				continue
			}
			if update.Message.From == nil || update.Message.From.ID != adminID {
				continue
			}

			command := update.Message.Command()
			switch command {
			case "start":
				if err := t.SendText(update.Message.Chat.ID, t.availableCommandsText()); err != nil {
					return err
				}
			case "post", "full", "summary", "image":
				var (
					chatID = update.Message.Chat.ID
					args   = update.Message.CommandArguments()
					spec   = commandSpecs[command]
				)

				go func() {
					if err := t.handleCommand(chatID, args, spec, f); err != nil {
						slog.Error("handle telegram command failed", "chat_id", chatID, "command", command, "error", err)
					}
				}()
			default:
				if err := t.SendText(update.Message.Chat.ID, "Unknown command. Use /start to see available commands."); err != nil {
					return err
				}
			}
		}
	}
}

func (t *Tg) availableCommandsText() string {
	lines := make([]string, 0, len(botCommands)+1)
	lines = append(lines, "Available commands:")
	for _, cmd := range botCommands {
		lines = append(lines, fmt.Sprintf("/%s - %s", cmd.Command, cmd.Description))
	}

	return strings.Join(lines, "\n")
}

func (t *Tg) handleCommand(chatID int64, args string, spec commandSpec, onRun func(chatID int64, urls []string, outputs notebooklm.Outputs) (notebooklm.Out, error)) error {
	urls := t.reurl.FindAllString(strings.TrimSpace(args), -1)
	if len(urls) == 0 {
		return t.SendText(chatID, "Usage: "+spec.Usage)
	}

	if err := t.SendText(chatID, "Accepted, processing..."); err != nil {
		return err
	}

	out, err := onRun(chatID, urls, spec.Outputs)
	if err != nil {
		return t.SendText(chatID, fmt.Sprintf("Failed: %v", err))
	}

	if spec.Outputs.Image {
		caption := ""
		var chunks []string
		if spec.Outputs.Report {
			caption, chunks = MarkdownToTelegramHTMLCaptionAndChunks(out.Report.Data, telegramPhotoCaptionLimit, 0)
		}
		if err := t.SendPhoto(chatID, out.Image.Data, out.Image.Path, caption); err != nil {
			return err
		}
		for _, chunk := range chunks {
			if err := t.SendHTML(chatID, chunk); err != nil {
				return err
			}
		}
	} else if spec.Outputs.Report {
		for _, chunk := range MarkdownToTelegramHTMLChunks(out.Report.Data, 0) {
			if err := t.SendHTML(chatID, chunk); err != nil {
				return err
			}
		}
	}

	if spec.Outputs.Audio && len(out.Audio.Data) > 0 {
		if err := t.SendAudio(chatID, out.Audio.Data, out.Audio.Path); err != nil {
			return err
		}
	}

	return nil
}

func (t *Tg) SendText(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := t.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}

	return nil
}

func (t *Tg) SendHTML(chatID int64, htmlText string) error {
	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.ParseMode = tgbotapi.ModeHTML
	_, err := t.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("send telegram html message: %w", err)
	}

	return nil
}

func (t *Tg) SendPhoto(chatID int64, image []byte, path, caption string) error {
	name := filepath.Base(strings.TrimSpace(path))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "poster.png"
	}

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileReader{
		Name:   name,
		Reader: bytes.NewReader(image),
	})
	if caption != "" {
		photo.Caption = caption
		photo.ParseMode = tgbotapi.ModeHTML
	}
	_, err := t.bot.Send(photo)
	if err != nil {
		return fmt.Errorf("send telegram photo: %w", err)
	}

	return nil
}

func (t *Tg) SendAudio(chatID int64, audio []byte, path string) error {
	name := filepath.Base(strings.TrimSpace(path))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "brief.mp3"
	}

	msg := tgbotapi.NewAudio(chatID, tgbotapi.FileReader{
		Name:   name,
		Reader: bytes.NewReader(audio),
	})
	_, err := t.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("send telegram audio: %w", err)
	}

	return nil
}
