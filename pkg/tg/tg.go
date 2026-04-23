package tg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Tg struct {
	bot      *tgbotapi.BotAPI
	reurl    *regexp.Regexp
	updateTO int
}

var botCommands = []tgbotapi.BotCommand{
	{Command: "start", Description: "show available commands"},
	{Command: "run", Description: "launch a pipeline for incoming sources"},
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

func (t *Tg) Run(ctx context.Context, adminID int64, f func(int64, []string) ([]byte, []byte, error)) error {
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

			switch update.Message.Command() {
			case "start":
				if err := t.SendText(update.Message.Chat.ID, t.availableCommandsText()); err != nil {
					return err
				}
			case "run":
				var (
					chatID = update.Message.Chat.ID
					args   = update.Message.CommandArguments()
				)

				go func() {
					if err := t.handleRun(chatID, args, f); err != nil {
						slog.Error("handle /run failed", "chat_id", chatID, "error", err)
					}
				}()
			default:
				if err := t.SendText(update.Message.Chat.ID, "Unknown command. Use /run <source-url>"); err != nil {
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

func (t *Tg) handleRun(chatID int64, args string, onRun func(chatID int64, urls []string) ([]byte, []byte, error)) error {
	urls := t.reurl.FindAllString(strings.TrimSpace(args), -1)
	if len(urls) == 0 {
		return t.SendText(chatID, "Usage: /run <source-url>")
	}

	if err := t.SendText(chatID, "Accepted, processing..."); err != nil {
		return err
	}

	image, report, err := onRun(chatID, urls)
	if err != nil {
		return t.SendText(chatID, fmt.Sprintf("Failed: %v", err))
	}

	caption, chunks := MarkdownToTelegramHTMLCaptionAndChunks(report, telegramPhotoCaptionLimit, 0)
	if err := t.SendPhoto(chatID, image, caption); err != nil {
		return err
	}

	for _, chunk := range chunks {
		if err := t.SendHTML(chatID, chunk); err != nil {
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

func (t *Tg) SendPhoto(chatID int64, image []byte, caption string) error {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileReader{
		Name:   "poster.png",
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
