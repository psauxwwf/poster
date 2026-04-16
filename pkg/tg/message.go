package tg

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

const telegramMessageLimit = 4096

func MarkdownToTelegramHTML(md []byte) []byte {
	src := []byte(toLines(string(md)))
	doc := goldmark.DefaultParser().Parse(text.NewReader(src))

	blocks := make([]string, 0, doc.ChildCount())
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		block := strings.TrimSpace(renderBlock(n, src))
		if block != "" {
			blocks = append(blocks, block)
		}
	}

	return []byte(strings.Join(blocks, "\n\n"))
}

func MarkdownToTelegramHTMLChunks(md []byte, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = telegramMessageLimit
	}

	htmlText := MarkdownToTelegramHTML(md)
	if strings.TrimSpace(string(htmlText)) == "" {
		return nil
	}

	blocks := strings.Split(string(htmlText), "\n\n")
	chunks := make([]string, 0, 4)
	cur := ""

	for _, block := range blocks {
		if block == "" {
			continue
		}

		if cur == "" {
			if runeLen(block) <= maxLen {
				cur = block
			} else {
				chunks = append(chunks, splitByRunes(block, maxLen)...)
			}
			continue
		}

		if runeLen(cur)+2+runeLen(block) <= maxLen {
			cur += "\n\n" + block
			continue
		}

		chunks = append(chunks, cur)
		if runeLen(block) <= maxLen {
			cur = block
		} else {
			chunks = append(chunks, splitByRunes(block, maxLen)...)
			cur = ""
		}
	}

	if cur != "" {
		chunks = append(chunks, cur)
	}

	return chunks
}

func MarkdownToTelegramHTMLCaptionAndChunks(md []byte, captionMaxLen, messageMaxLen int) (string, []string) {
	if captionMaxLen <= 0 {
		captionMaxLen = 1024
	}
	if messageMaxLen <= 0 {
		messageMaxLen = telegramMessageLimit
	}

	htmlText := strings.TrimSpace(string(MarkdownToTelegramHTML(md)))
	if htmlText == "" {
		return "", nil
	}

	blocks := splitNonEmptyBlocks(htmlText)
	if len(blocks) == 0 {
		return "", nil
	}

	captionBlocks := make([]string, 0, len(blocks))
	captionLen := 0
	nextBlock := 0

	for i, block := range blocks {
		addLen := runeLen(block)
		if len(captionBlocks) > 0 {
			addLen += 2
		}

		if captionLen+addLen > captionMaxLen {
			break
		}

		captionBlocks = append(captionBlocks, block)
		captionLen += addLen
		nextBlock = i + 1
	}

	caption := strings.Join(captionBlocks, "\n\n")
	if nextBlock >= len(blocks) {
		return caption, nil
	}

	chunks := joinBlocksToChunks(blocks[nextBlock:], messageMaxLen)
	return caption, chunks
}

func renderBlock(n ast.Node, src []byte) string {
	switch v := n.(type) {
	case *ast.Heading:
		return "<b>" + renderInlines(v, src) + "</b>"
	case *ast.Paragraph:
		return renderInlines(v, src)
	case *ast.Blockquote:
		parts := make([]string, 0, v.ChildCount())
		for c := v.FirstChild(); c != nil; c = c.NextSibling() {
			part := strings.TrimSpace(renderBlock(c, src))
			if part != "" {
				parts = append(parts, part)
			}
		}
		return "<blockquote>" + strings.Join(parts, "\n") + "</blockquote>"
	case *ast.List:
		return renderList(v, src)
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		return renderCodeBlock(n, src)
	default:
		if n.FirstChild() != nil {
			return renderInlines(n, src)
		}
		return html.EscapeString(string(n.Text(src)))
	}
}

func renderList(list *ast.List, src []byte) string {
	lines := make([]string, 0, list.ChildCount())
	idx := list.Start
	if idx <= 0 {
		idx = 1
	}

	for n := list.FirstChild(); n != nil; n = n.NextSibling() {
		item, ok := n.(*ast.ListItem)
		if !ok {
			continue
		}

		text := strings.TrimSpace(renderListItem(item, src))
		if text == "" {
			continue
		}

		prefix := "• "
		if list.IsOrdered() {
			prefix = fmt.Sprintf("%d) ", idx)
			idx++
		}
		lines = append(lines, prefix+text)
	}

	return strings.Join(lines, "\n")
}

func renderListItem(item *ast.ListItem, src []byte) string {
	parts := make([]string, 0, item.ChildCount())
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Paragraph:
			text := strings.TrimSpace(renderInlines(v, src))
			if text != "" {
				parts = append(parts, text)
			}
		case *ast.List:
			nested := strings.TrimSpace(renderList(v, src))
			if nested != "" {
				parts = append(parts, "\n"+nested)
			}
		default:
			text := strings.TrimSpace(renderBlock(c, src))
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.ReplaceAll(strings.Join(parts, " "), " \n", "\n")
}

func renderCodeBlock(n ast.Node, src []byte) string {
	lines := n.Lines()
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write((&seg).Value(src))
	}
	return "<pre><code>" + html.EscapeString(strings.TrimRight(b.String(), "\n")) + "</code></pre>"
}

func renderInlines(parent ast.Node, src []byte) string {
	var b strings.Builder
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		b.WriteString(renderInline(c, src))
	}
	return b.String()
}

func renderInline(n ast.Node, src []byte) string {
	switch v := n.(type) {
	case *ast.Text:
		text := html.EscapeString(string(v.Segment.Value(src)))
		if v.HardLineBreak() || v.SoftLineBreak() {
			return text + "\n"
		}
		return text
	case *ast.String:
		return html.EscapeString(string(v.Value))
	case *ast.Emphasis:
		tag := "i"
		if v.Level == 2 {
			tag = "b"
		}
		return "<" + tag + ">" + renderInlines(v, src) + "</" + tag + ">"
	case *ast.CodeSpan:
		return "<code>" + html.EscapeString(string(v.Text(src))) + "</code>"
	case *ast.Link:
		href := html.EscapeString(string(v.Destination))
		label := renderInlines(v, src)
		if strings.TrimSpace(label) == "" {
			label = href
		}
		return `<a href="` + href + `">` + label + "</a>"
	default:
		if n.FirstChild() != nil {
			return renderInlines(n, src)
		}
		return html.EscapeString(string(n.Text(src)))
	}
}

func toLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func splitByRunes(s string, maxLen int) []string {
	if maxLen <= 0 {
		return []string{s}
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return []string{s}
	}

	parts := make([]string, 0, len(runes)/maxLen+1)
	for start := 0; start < len(runes); start += maxLen {
		end := min(start+maxLen, len(runes))
		parts = append(parts, string(runes[start:end]))
	}

	return parts
}

func splitNonEmptyBlocks(s string) []string {
	raw := strings.Split(s, "\n\n")
	blocks := make([]string, 0, len(raw))

	for _, block := range raw {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		blocks = append(blocks, block)
	}

	return blocks
}

func joinBlocksToChunks(blocks []string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = telegramMessageLimit
	}

	chunks := make([]string, 0, 4)
	cur := ""

	for _, block := range blocks {
		if cur == "" {
			if runeLen(block) <= maxLen {
				cur = block
				continue
			}

			chunks = append(chunks, splitByRunes(block, maxLen)...)
			continue
		}

		if runeLen(cur)+2+runeLen(block) <= maxLen {
			cur += "\n\n" + block
			continue
		}

		chunks = append(chunks, cur)
		if runeLen(block) <= maxLen {
			cur = block
			continue
		}

		chunks = append(chunks, splitByRunes(block, maxLen)...)
		cur = ""
	}

	if cur != "" {
		chunks = append(chunks, cur)
	}

	return chunks
}
