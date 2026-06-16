package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"vendor-onboarding-api/internal/scanner"
)

var defaultScanner = scanner.NewDefaultScanner()

func SetVirusScanner(s scanner.Scanner) {
	defaultScanner = s
}

type ProcessingStep string

const (
	StepVirusScan     ProcessingStep = "virus_scan"
	StepImageCrop     ProcessingStep = "image_crop"
	StepPDFSanitize   ProcessingStep = "pdf_sanitize"
	StepHashCompute   ProcessingStep = "hash_compute"
)

type ProcessOptions struct {
	SkipVirusScan   bool
	SkipImageCrop   bool
	SkipPDFSanitize bool
	MaxImageWidth   int
	MaxImageHeight  int
	ImageQuality    int
	ScannerTimeout  time.Duration
}

func DefaultProcessOptions() ProcessOptions {
	return ProcessOptions{
		SkipVirusScan:   false,
		SkipImageCrop:   false,
		SkipPDFSanitize: false,
		MaxImageWidth:   1920,
		MaxImageHeight:  1920,
		ImageQuality:    85,
		ScannerTimeout:  10 * time.Second,
	}
}

type ProcessResult struct {
	Skipped []ProcessingStep `json:"skipped"`
	Applied []ProcessingStep `json:"applied"`
	NewSize int64            `json:"new_size"`
	WasInfected bool         `json:"was_infected"`
	CropApplied bool         `json:"crop_applied"`
	Threats   []string       `json:"threats"`
	ScanInfo  *scanner.ScanResult `json:"scan_info,omitempty"`
}

func ProcessFileChain(ctx context.Context, srcPath string, ext string, opts ProcessOptions) (*ProcessResult, error) {
	result := &ProcessResult{}
	ext = strings.ToLower(ext)

	if !opts.SkipVirusScan {
		scanCtx := ctx
		var cancel context.CancelFunc
		if opts.ScannerTimeout > 0 {
			scanCtx, cancel = context.WithTimeout(ctx, opts.ScannerTimeout)
			defer cancel()
		}
		scanResult, err := defaultScanner.Scan(scanCtx, srcPath, ext)
		if err != nil {
			return nil, fmt.Errorf("virus scan failed: %w", err)
		}
		result.ScanInfo = scanResult
		if !scanResult.Clean {
			result.WasInfected = true
			result.Threats = scanResult.Threats
			return result, fmt.Errorf("virus detected: %v", scanResult.Threats)
		}
		result.Applied = append(result.Applied, StepVirusScan)
	} else {
		result.Skipped = append(result.Skipped, StepVirusScan)
	}

	isImage := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
	isPDF := ext == ".pdf"

	if isImage[ext] && !opts.SkipImageCrop {
		dstPath := srcPath + ".processed" + ext
		applied, err := CropAndResizeImage(srcPath, dstPath,
			opts.MaxImageWidth, opts.MaxImageHeight, opts.ImageQuality)
		if err != nil {
			return result, fmt.Errorf("image crop failed: %w", err)
		}
		if applied {
			_ = os.Rename(dstPath, srcPath)
			result.CropApplied = true
			result.Applied = append(result.Applied, StepImageCrop)
			if info, err := os.Stat(srcPath); err == nil {
				result.NewSize = info.Size()
			}
		} else {
			_ = os.Remove(dstPath)
			result.Skipped = append(result.Skipped, StepImageCrop)
		}
	}

	if isPDF && !opts.SkipPDFSanitize {
		dstPath := srcPath + ".sanitized.pdf"
		applied, err := SanitizePDF(srcPath, dstPath)
		if err != nil {
			return result, fmt.Errorf("pdf sanitize failed: %w", err)
		}
		if applied {
			_ = os.Rename(dstPath, srcPath)
			result.Applied = append(result.Applied, StepPDFSanitize)
			if info, err := os.Stat(srcPath); err == nil {
				result.NewSize = info.Size()
			}
		} else {
			_ = os.Remove(dstPath)
			result.Skipped = append(result.Skipped, StepPDFSanitize)
		}
	}

	return result, nil
}

var virusSignaturePatterns = []*regexp.Regexp{
	regexp.MustCompile(`<script[\s\S]*?>[\s\S]*?</script>`),
	regexp.MustCompile(`eval\s*\(`),
	regexp.MustCompile(`exec\s*\(`),
	regexp.MustCompile(`shell_exec\s*\(`),
	regexp.MustCompile(`base64_decode\s*\(`),
	regexp.MustCompile(`<\?php`),
	regexp.MustCompile(`<%\s*`),
	regexp.MustCompile(`autoexeC`),
	regexp.MustCompile(`onerror\s*=`),
	regexp.MustCompile(`onload\s*=`),
	regexp.MustCompile(`javascript:`),
}

func ScanVirus(path string, ext string) (bool, []string, error) {
	result, err := defaultScanner.Scan(context.Background(), path, ext)
	if err != nil {
		return false, nil, err
	}
	return !result.Clean, result.Threats, nil
}

func ScanVirusWithContext(ctx context.Context, path string, ext string) (*scanner.ScanResult, error) {
	return defaultScanner.Scan(ctx, path, ext)
}

func isScriptExtension(ext string) bool {
	ext = strings.ToLower(ext)
	blocked := map[string]bool{
		".exe": true, ".bat": true, ".cmd": true, ".sh": true,
		".js":  true, ".vbs": true, ".ps1": true, ".jar": true,
		".com": true, ".pif": true, ".scr": true, ".msi": true,
		".php": true, ".jsp": true, ".asp": true, ".aspx": true,
		".py":  true, ".rb":  true, ".pl":  true,
	}
	return blocked[ext]
}

func detectMagic(head []byte) string {
	if len(head) < 4 {
		return "unknown"
	}
	switch {
	case bytes.HasPrefix(head, []byte("MZ")):
		return "exe"
	case bytes.HasPrefix(head, []byte{0x7f, 'E', 'L', 'F'}):
		return "elf"
	case bytes.HasPrefix(head, []byte{0xFE, 0xED, 0xFA}):
		return "macho"
	case bytes.HasPrefix(head, []byte{0xCF, 0xFA, 0xED, 0xFE}):
		return "macho"
	case bytes.HasPrefix(head, []byte("@echo")) || bytes.HasPrefix(head, []byte("@ECHO")):
		return "batch"
	case bytes.HasPrefix(head, []byte("#!/")):
		return "shell"
	case bytes.HasPrefix(head, []byte{0x89, 'P', 'N', 'G'}):
		return "png"
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return "jpeg"
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return "pdf"
	case bytes.HasPrefix(head, []byte("GIF8")):
		return "gif"
	}
	return "unknown"
}

func CropAndResizeImage(srcPath, dstPath string, maxW, maxH, quality int) (bool, error) {
	ext := strings.ToLower(filepath.Ext(srcPath))
	f, err := os.Open(srcPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var img image.Image
	switch ext {
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(f)
	case ".png":
		img, err = png.Decode(f)
	default:
		return false, fmt.Errorf("unsupported image type for crop: %s", ext)
	}
	if err != nil {
		return false, err
	}

	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()

	needResize := w > maxW || h > maxH
	if !needResize {
		return false, nil
	}

	newW, newH := calculateResize(w, h, maxW, maxH)
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	out, err := os.Create(dstPath)
	if err != nil {
		return false, err
	}
	defer out.Close()

	switch ext {
	case ".jpg", ".jpeg":
		if quality <= 0 || quality > 100 {
			quality = 85
		}
		err = jpeg.Encode(out, dst, &jpeg.Options{Quality: quality})
	case ".png":
		err = png.Encode(out, dst)
	}
	return true, err
}

func calculateResize(w, h, maxW, maxH int) (int, int) {
	ratioW := float64(maxW) / float64(w)
	ratioH := float64(maxH) / float64(h)
	ratio := ratioW
	if ratioH < ratio {
		ratio = ratioH
	}
	return int(float64(w) * ratio), int(float64(h) * ratio)
}

var pdfHeaderRegex = regexp.MustCompile(`%PDF-[0-9]+\.[0-9]+`)

func SanitizePDF(srcPath, dstPath string) (bool, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	head := make([]byte, 8)
	n, _ := f.Read(head)
	if !pdfHeaderRegex.Match(head[:n]) {
		return false, errors.New("not a valid PDF file")
	}
	_, _ = f.Seek(0, io.SeekStart)

	data, err := io.ReadAll(f)
	if err != nil {
		return false, err
	}

	modified := false
	metadataTags := []string{
		"/Title(", "/Author(", "/Subject(", "/Keywords(",
		"/Creator(", "/Producer(", "/CreationDate(", "/ModDate(",
		"/Trapped(",
		"/JavaScript", "/JS(", "/OpenAction(", "/AA(",
		"/Launch(", "/EmbeddedFiles", "/Names[",
	}

	cleaned := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		matched := false
		for _, tag := range metadataTags {
			if i+len(tag) <= len(data) && bytes.Equal([]byte(tag), data[i:i+len(tag)]) {
				end := findObjectEnd(data, i)
				i = end
				matched = true
				modified = true
				break
			}
		}
		if !matched {
			cleaned = append(cleaned, data[i])
			i++
		}
	}

	streamRegex := regexp.MustCompile(`/Filter\s*/\w*JS\w*`)
	if streamRegex.Match(cleaned) {
		cleaned = streamRegex.ReplaceAllLiteral(cleaned, []byte("/Filter/FlateDecode"))
		modified = true
	}

	if !modified {
		return false, nil
	}

	if err := os.WriteFile(dstPath, cleaned, 0644); err != nil {
		return false, err
	}
	return true, nil
}

func findObjectEnd(data []byte, start int) int {
	depth := 0
	i := start
	inStr := false
	escaped := false
	for i < len(data) {
		c := data[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if c == '\\' {
			escaped = true
			i++
			continue
		}
		if c == '(' && !inStr {
			depth++
			inStr = true
			i++
			continue
		}
		if c == ')' && inStr {
			depth--
			inStr = false
			i++
			if depth <= 0 {
				return i
			}
			continue
		}
		if i+2 <= len(data) && bytes.Equal([]byte("<<"), data[i:i+2]) {
			depth++
			i += 2
			continue
		}
		if i+2 <= len(data) && bytes.Equal([]byte(">>"), data[i:i+2]) {
			depth--
			i += 2
			if depth <= 0 {
				return i
			}
			continue
		}
		i++
	}
	return i
}
