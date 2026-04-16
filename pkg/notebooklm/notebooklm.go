package notebooklm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"poster/pkg/cmd"
)

type createNotebookResponse struct {
	Notebook struct {
		ID string `json:"id"`
	} `json:"notebook"`
}

type addSourceResponse struct {
	Source struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"source"`
}

type generateArtifactResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

type listNotebooksResponse struct {
	Notebooks []struct {
		ID string `json:"id"`
	} `json:"notebooks"`
}

type Notebook struct {
	ID  string
	Raw map[string]any
}

type Source struct {
	ID    string
	Title string
	Raw   map[string]any
}

type Artifact struct {
	ID  string
	Raw map[string]any
}

type NotebookLM struct {
	bin string
}

type YouTubePipelineOutput struct {
	Image  []byte
	Report []byte
}

func New(binary string) (*NotebookLM, error) {
	if strings.TrimSpace(binary) == "" {
		binary = "notebooklm"
	}

	n := &NotebookLM{bin: binary}
	if _, _, err := n.run(context.Background(), "language", "set", "ru"); err != nil {
		return nil, fmt.Errorf("set notebooklm language: %w", err)
	}

	return n, nil
}

func (n *NotebookLM) Status(ctx context.Context) error {
	_, _, err := n.run(ctx, "status")
	if err != nil {
		return err
	}

	return nil
}

func (n *NotebookLM) List(ctx context.Context) (map[string]any, error) {
	data, _, err := n.runJSON(ctx, "list", "--json")
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (n *NotebookLM) ListNotebookIDs(ctx context.Context) ([]string, error) {
	data, err := n.List(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var parsed listNotebooksResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(parsed.Notebooks))
	for _, notebook := range parsed.Notebooks {
		if notebook.ID != "" {
			ids = append(ids, notebook.ID)
		}
	}

	return ids, nil
}

func (n *NotebookLM) DeleteNotebook(ctx context.Context, notebookID string) error {
	attempts := [][]string{
		{"delete", "-n", notebookID, "-y"},
		{"delete", "--notebook", notebookID, "--yes"},
	}

	var errs []error
	for _, args := range attempts {
		_, _, err := n.run(ctx, args...)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (n *NotebookLM) CreateNotebook(ctx context.Context, title string) (Notebook, error) {
	resp, raw, err := n.runJSON(ctx, "create", title, "--json")
	if err != nil {
		return Notebook{}, err
	}

	var parsed createNotebookResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Notebook{}, fmt.Errorf("parse create notebook response: %w", err)
	}

	notebookID := strings.TrimSpace(parsed.Notebook.ID)
	if notebookID == "" {
		return Notebook{}, fmt.Errorf("notebook id not found in create response")
	}

	return Notebook{ID: notebookID, Raw: resp}, nil
}

func (n *NotebookLM) RenameNotebook(ctx context.Context, notebookID, newTitle string) error {
	attempts := [][]string{
		{"rename", newTitle, "--notebook", notebookID},
		{"rename", "--notebook", notebookID, newTitle},
		{"notebook", "rename", notebookID, newTitle},
		{"notebook", "rename", newTitle, "--notebook", notebookID},
	}

	var errs []error
	for _, args := range attempts {
		_, _, callErr := n.run(ctx, args...)
		if callErr == nil {
			return nil
		}
		errs = append(errs, callErr)
	}

	return errors.Join(errs...)
}

func (n *NotebookLM) AddSource(ctx context.Context, notebookID, url string) (Source, error) {
	resp, raw, err := n.runJSON(ctx, "source", "add", url, "--notebook", notebookID, "--json")
	if err != nil {
		return Source{}, err
	}

	var parsed addSourceResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Source{}, fmt.Errorf("parse add source response: %w", err)
	}

	sourceID := strings.TrimSpace(parsed.Source.ID)
	if sourceID == "" {
		return Source{}, fmt.Errorf("source id not found in source add response")
	}

	title := strings.TrimSpace(parsed.Source.Title)
	if title == "" {
		title = url
	}

	return Source{ID: sourceID, Title: title, Raw: resp}, nil
}

func (n *NotebookLM) WaitSource(ctx context.Context, notebookID, sourceID string, timeout time.Duration) error {
	seconds := int(timeout.Seconds())
	if seconds <= 0 {
		seconds = 1
	}

	if _, _, err := n.run(ctx, "source", "wait", sourceID, "-n", notebookID, "--timeout", fmt.Sprintf("%d", seconds)); err != nil {
		return fmt.Errorf("source wait failed: %w", err)
	}

	return nil
}

func (n *NotebookLM) GenerateReport(
	ctx context.Context,
	notebookID, format string, prompt ...string,
) (Artifact, error) {
	if strings.TrimSpace(format) == "" {
		format = "blog-post"
	}
	args := []string{
		"generate",
		"report",
		"--format",
		format,
		"--notebook",
		notebookID,
		"--json",
	}
	if len(prompt) > 0 {
		args = append(args, prompt[0])
	}

	resp, raw, err := n.runJSON(
		ctx,
		args...,
	)
	if err != nil {
		return Artifact{}, err
	}

	var parsed generateArtifactResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Artifact{}, fmt.Errorf("parse generate report response: %w", err)
	}

	artifactID := strings.TrimSpace(parsed.TaskID)
	if artifactID == "" {
		return Artifact{}, fmt.Errorf("artifact id not found in report response")
	}

	return Artifact{ID: artifactID, Raw: resp}, nil
}

func (n *NotebookLM) GenerateInfographic(
	ctx context.Context,
	notebookID, detail, style string, prompt ...string,
) (Artifact, error) {
	if strings.TrimSpace(detail) == "" {
		detail = "detailed"
	}
	if strings.TrimSpace(style) == "" {
		style = "sketch-note"
	}
	args := []string{
		"generate",
		"infographic",
		"--detail",
		detail,
		"--style",
		style,
		"--notebook",
		notebookID,
		"--json",
	}
	if len(prompt) > 0 {
		args = append(args, prompt[0])
	}

	resp, raw, err := n.runJSON(
		ctx,
		args...,
	)
	if err != nil {
		return Artifact{}, err
	}

	var parsed generateArtifactResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Artifact{}, fmt.Errorf("parse generate infographic response: %w", err)
	}

	artifactID := strings.TrimSpace(parsed.TaskID)
	if artifactID == "" {
		return Artifact{}, fmt.Errorf("artifact id not found in infographic response")
	}

	return Artifact{ID: artifactID, Raw: resp}, nil
}

func (n *NotebookLM) WaitArtifact(ctx context.Context, notebookID, artifactID string, timeout time.Duration) error {
	seconds := int(timeout.Seconds())
	if seconds <= 0 {
		seconds = 1
	}

	_, _, err := n.run(ctx, "artifact", "wait", artifactID, "-n", notebookID, "--timeout", fmt.Sprintf("%d", seconds))
	if err != nil {
		return fmt.Errorf("artifact wait failed: %w", err)
	}

	return nil
}

func (n *NotebookLM) DownloadReport(ctx context.Context, notebookID, artifactID, outputPath string) error {
	if _, _, err := n.run(ctx, "download", "report", outputPath, "-a", artifactID, "-n", notebookID); err != nil {
		return err
	}

	return nil
}

func (n *NotebookLM) DownloadInfographic(ctx context.Context, notebookID, artifactID, outputPath string) error {
	if _, _, err := n.run(ctx, "download", "infographic", outputPath, "-a", artifactID, "-n", notebookID); err != nil {
		return err
	}

	return nil
}

func (n *NotebookLM) RunYouTubePipeline(
	ctx context.Context,
	url,
	outDir string,
	timeoutSource,
	timeoutArtifact time.Duration,
	reportPrompt,
	infographicStyle string,
) (YouTubePipelineOutput, error) {
	if strings.TrimSpace(url) == "" {
		return YouTubePipelineOutput{}, fmt.Errorf("empty url")
	}

	if err := n.Status(ctx); err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("notebooklm is not ready: %w; run `notebooklm login`", err)
	}

	notebookName := fmt.Sprintf("yt-import-%s", time.Now().UTC().Format("20060102-150405"))
	notebook, err := n.CreateNotebook(ctx, notebookName)
	if err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("create notebook: %w", err)
	}
	slog.Info("notebook created", "notebook_id", notebook.ID)

	source, err := n.AddSource(ctx, notebook.ID, url)
	if err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("add source: %w", err)
	}
	slog.Info("source added", "source_id", source.ID)

	if err := n.WaitSource(
		ctx,
		notebook.ID,
		source.ID,
		timeoutSource,
	); err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("source wait failed: %w; manual retry: notebooklm source wait %s -n %s --timeout %d", err, source.ID, notebook.ID, int(timeoutSource.Seconds()))
	}

	reportArtifact, err := n.GenerateReport(ctx, notebook.ID, "blog-post", reportPrompt)
	if err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("generate report: %w", err)
	}
	infographicArtifact, err := n.GenerateInfographic(
		ctx,
		notebook.ID,
		"detailed",
		"sketch-note",
		infographicStyle,
	)
	if err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("generate infographic: %w", err)
	}

	if err := n.WaitArtifact(
		ctx,
		notebook.ID,
		reportArtifact.ID,
		timeoutArtifact,
	); err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("report wait failed: %w; manual retry: notebooklm artifact wait %s -n %s --timeout %d", err, reportArtifact.ID, notebook.ID, int(timeoutArtifact.Seconds()))
	}
	slog.Info("report artifact created", "report_artifact_id", reportArtifact.ID)

	if err := n.WaitArtifact(
		ctx,
		notebook.ID,
		infographicArtifact.ID,
		timeoutArtifact,
	); err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("infographic wait failed: %w; manual retry: notebooklm artifact wait %s -n %s --timeout %d", err, infographicArtifact.ID, notebook.ID, int(timeoutArtifact.Seconds()))
	}
	slog.Info("infographic artifact created", "infographic_artifact_id", infographicArtifact.ID)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("create output dir: %w", err)
	}

	baseName := sanitizeTitle(source.Title, 100, notebook.ID)

	reportPath, err := uniquePath(outDir, baseName, ".md")
	if err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("pick report path: %w", err)
	}

	infographicPath, err := uniquePath(outDir, baseName, ".png")
	if err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("pick infographic path: %w", err)
	}

	if err := n.DownloadReport(ctx, notebook.ID, reportArtifact.ID, reportPath); err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("download report: %w", err)
	}

	if err := n.DownloadInfographic(ctx, notebook.ID, infographicArtifact.ID, infographicPath); err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("download infographic: %w", err)
	}

	if err := n.RenameNotebook(ctx, notebook.ID, baseName); err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("rename notebook: %w", err)
	}

	report, err := os.ReadFile(reportPath)
	if err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("open report md: %w", err)
	}

	image, err := os.ReadFile(infographicPath)
	if err != nil {
		return YouTubePipelineOutput{}, fmt.Errorf("open infographic png: %w", err)
	}

	return YouTubePipelineOutput{
		Image:  image,
		Report: report,
	}, nil
}

func (n *NotebookLM) runJSON(ctx context.Context, args ...string) (map[string]any, []byte, error) {
	stdout, stderr, err := n.run(ctx, args...)
	if err != nil {
		return nil, nil, err
	}

	if strings.TrimSpace(stdout) == "" {
		return nil, nil, fmt.Errorf("empty output for `%s %s`", n.bin, strings.Join(args, " "))
	}

	raw := []byte(stdout)

	var data map[string]any
	if parseErr := json.Unmarshal(raw, &data); parseErr != nil {
		return nil, nil, fmt.Errorf("invalid json for `%s %s`: %w; stdout=%q stderr=%q", n.bin, strings.Join(args, " "), parseErr, stdout, stderr)
	}

	return data, raw, nil
}

func (n *NotebookLM) run(ctx context.Context, args ...string) (string, string, error) {
	command := n.bin + " " + strings.Join(args, " ")

	stdoutText, stderrText, err := cmd.Run(ctx, n.bin, args...)

	if err != nil {
		slog.Debug("notebooklm command failed", "command", command, "stdout", stdoutText, "stderr", stderrText, "error", err)
		return stdoutText, stderrText, err
	}

	slog.Debug("notebooklm command response", "command", command, "stdout", stdoutText, "stderr", stderrText)

	return stdoutText, stderrText, nil
}

var (
	forbiddenChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
	spaces         = regexp.MustCompile(`\s+`)
)

func sanitizeTitle(input string, maxLen int, fallback string) string {
	name := strings.TrimSpace(input)
	name = forbiddenChars.ReplaceAllString(name, "")
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
