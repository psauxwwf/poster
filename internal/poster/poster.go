package poster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"poster/pkg/notebooklm"
)

type Poster struct {
	notebooklm *notebooklm.NotebookLM
	outDir     string

	timeoutSource   time.Duration
	timeoutArtifact time.Duration
}

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

func (p *Poster) Run(url string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("empty url")
	}

	ctx := context.Background()

	if err := p.notebooklm.Status(ctx); err != nil {
		return fmt.Errorf("notebooklm is not ready: %w; run `notebooklm login`", err)
	}

	notebookName := fmt.Sprintf("yt-import-%s", time.Now().UTC().Format("20060102-150405"))
	notebook, err := p.notebooklm.CreateNotebook(ctx, notebookName)
	if err != nil {
		return fmt.Errorf("create notebook: %w", err)
	}
	fmt.Printf("notebook_id=%s\n", notebook.ID)

	source, err := p.notebooklm.AddSource(ctx, notebook.ID, url)
	if err != nil {
		return fmt.Errorf("add source: %w", err)
	}
	fmt.Printf("source_id=%s\n", source.ID)

	if err := p.notebooklm.WaitSource(ctx, notebook.ID, source.ID, p.timeoutSource); err != nil {
		return fmt.Errorf("source wait failed: %w; manual retry: notebooklm source wait %s -n %s --timeout %d", err, source.ID, notebook.ID, int(p.timeoutSource.Seconds()))
	}

	reportArtifact, err := p.notebooklm.GenerateReport(ctx, notebook.ID, "blog-post")
	if err != nil {
		return fmt.Errorf("generate report: %w", err)
	}
	fmt.Printf("report_artifact_id=%s\n", reportArtifact.ID)

	if err := p.notebooklm.WaitArtifact(ctx, notebook.ID, reportArtifact.ID, p.timeoutArtifact); err != nil {
		return fmt.Errorf("report wait failed: %w; manual retry: notebooklm artifact wait %s -n %s --timeout %d", err, reportArtifact.ID, notebook.ID, int(p.timeoutArtifact.Seconds()))
	}

	infographicArtifact, err := p.notebooklm.GenerateInfographic(ctx, notebook.ID, "detailed", "sketch-note")
	if err != nil {
		return fmt.Errorf("generate infographic: %w", err)
	}
	fmt.Printf("infographic_artifact_id=%s\n", infographicArtifact.ID)

	if err := p.notebooklm.WaitArtifact(ctx, notebook.ID, infographicArtifact.ID, p.timeoutArtifact); err != nil {
		return fmt.Errorf("infographic wait failed: %w; manual retry: notebooklm artifact wait %s -n %s --timeout %d", err, infographicArtifact.ID, notebook.ID, int(p.timeoutArtifact.Seconds()))
	}

	if err := os.MkdirAll(p.outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	fallbackID := source.ID
	if fallbackID == "" {
		fallbackID = notebook.ID
	}

	baseName := sanitizeTitle(source.Title, 100, "youtube-"+safeIDPart(fallbackID, 12))
	reportPath, err := uniquePath(p.outDir, baseName, ".md")
	if err != nil {
		return fmt.Errorf("pick report path: %w", err)
	}

	infographicPath, err := uniquePath(p.outDir, baseName, ".png")
	if err != nil {
		return fmt.Errorf("pick infographic path: %w", err)
	}

	if err := p.notebooklm.DownloadReport(ctx, notebook.ID, reportArtifact.ID, reportPath); err != nil {
		return fmt.Errorf("download report: %w", err)
	}

	if err := p.notebooklm.DownloadInfographic(ctx, notebook.ID, infographicArtifact.ID, infographicPath); err != nil {
		return fmt.Errorf("download infographic: %w", err)
	}

	if err := p.notebooklm.RenameNotebook(ctx, notebook.ID, baseName); err != nil {
		return fmt.Errorf("rename notebook: %w", err)
	}

	fmt.Printf("output_report=%s\n", reportPath)
	fmt.Printf("output_infographic=%s\n", infographicPath)
	fmt.Printf("notebook_title=%s\n", baseName)

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

var (
	forbiddenChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
	spaces         = regexp.MustCompile(`\s+`)
	idUnsafe       = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
)

func sanitizeTitle(input string, maxLen int, fallback string) string {
	name := strings.TrimSpace(input)
	name = forbiddenChars.ReplaceAllString(name, "-")
	name = spaces.ReplaceAllString(name, " ")
	name = strings.Trim(name, " .")

	if maxLen > 0 && utf8.RuneCountInString(name) > maxLen {
		runes := []rune(name)
		name = string(runes[:maxLen])
		name = strings.Trim(name, " .")
	}

	if name == "" {
		name = strings.TrimSpace(fallback)
	}
	if name == "" {
		name = "youtube"
	}

	return name
}

func safeIDPart(raw string, maxLen int) string {
	text := idUnsafe.ReplaceAllString(strings.TrimSpace(raw), "")
	if maxLen > 0 && len(text) > maxLen {
		text = text[:maxLen]
	}
	if text == "" {
		return "id"
	}

	return text
}

func uniquePath(dir, baseName, ext string) (string, error) {
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	first := filepath.Join(dir, baseName+ext)
	if ok, err := exists(first); err != nil {
		return "", err
	} else if !ok {
		return first, nil
	}

	for idx := 2; ; idx++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", baseName, idx, ext))
		ok, err := exists(candidate)
		if err != nil {
			return "", err
		}
		if !ok {
			return candidate, nil
		}
	}
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, err
}
