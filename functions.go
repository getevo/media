package media

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/gabriel-vasile/mimetype"
	"github.com/getevo/evo/v2"
	"github.com/getevo/evo/v2/lib/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unicode"
)

// IsFFMpegInstalled checks if ffmpeg is installed and available in PATH
func IsFFMpegInstalled() bool {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", "ffmpeg")
	} else {
		cmd = exec.Command("which", "ffmpeg")
	}

	err := cmd.Run()
	return err == nil
}

// IsFFProbeInstalled checks if ffmpeg is installed and available in PATH
func IsFFProbeInstalled() bool {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", "ffprobe")
	} else {
		cmd = exec.Command("which", "ffprobe")
	}

	err := cmd.Run()
	return err == nil
}

// GetVideoDuration returns the duration of the video in seconds using ffprobe
func GetVideoDuration(inputPath string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", inputPath)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get video duration: %w", err)
	}
	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse video duration: %w", err)
	}
	return duration, nil
}

// CreateMultipartFileHeader creates a multipart.FileHeader from a base64 image string and a file name
func CreateMultipartFileHeader(base64Data string, fileName string) (*multipart.FileHeader, error) {
	// Remove base64 header if present
	if idx := strings.Index(base64Data, "base64,"); idx != -1 {
		base64Data = base64Data[idx+7:]
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	// Prepare header
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+fileName+`"`)
	h.Set("Content-Type", "application/octet-stream")

	// Create part
	part, err := writer.CreatePart(h)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(part, bytes.NewReader(decoded)); err != nil {
		return nil, err
	}

	_ = writer.Close()

	// Parse multipart data
	reader := multipart.NewReader(&b, writer.Boundary())
	form, err := reader.ReadForm(int64(len(b.Bytes())))
	if err != nil {
		return nil, err
	}

	files := form.File["file"]
	if len(files) == 0 {
		return nil, io.EOF
	}

	return files[0], nil
}

// NormalizeFileName converts a UTF-8 filename/path to a safe ASCII version
func NormalizeFileName(input string) string {
	// Convert to lower case
	input = strings.ToLower(input)

	// Replace spaces with underscore
	input = strings.ReplaceAll(input, " ", "_")

	// Normalize Unicode to ASCII approximation (remove accents, etc.)
	var sb strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_') // Replace disallowed chars with underscore
		}
	}

	// Reduce consecutive underscores to a single underscore
	result := sb.String()
	re := regexp.MustCompile(`_+`)
	result = re.ReplaceAllString(result, "_")

	// Trim leading and trailing underscores
	result = strings.Trim(result, "_")

	return result
}

// DetectFileType detects file category and MIME type from either *multipart.FileHeader or *os.File
func DetectFileType(input interface{}) (FileInfo, error) {
	var (
		file     multipart.File
		size     int64
		err      error
		fileInfo os.FileInfo
	)

	switch v := input.(type) {
	case string:
		var f *os.File
		f, err = os.Open(v)
		if err != nil {
			return FileInfo{}, fmt.Errorf("failed to open file: %w", err)
		}
		file = f
		fileInfo, err = f.Stat()
		if err != nil {
			f.Close()
			return FileInfo{}, fmt.Errorf("failed to stat file: %w", err)
		}
		size = fileInfo.Size()

	case *os.File:
		file = v
		fileInfo, err = v.Stat()
		if err != nil {
			return FileInfo{}, fmt.Errorf("failed to stat file: %w", err)
		}
		size = fileInfo.Size()

	case *multipart.FileHeader:
		size = v.Size
		file, err = v.Open()
		if err != nil {
			return FileInfo{}, fmt.Errorf("failed to open multipart file: %w", err)
		}

	case multipart.File:
		file = v
		// Try to get the size by checking if it's an *os.File
		if f, ok := v.(*os.File); ok {
			fileInfo, err = f.Stat()
			if err == nil {
				size = fileInfo.Size()
			}
		}
		// If not possible, size will be 0

	default:
		return FileInfo{}, fmt.Errorf("unsupported file type: %T", input)
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return FileInfo{}, fmt.Errorf("failed to read file: %w", err)
	}

	mimeType := mimetype.Detect(buffer[:n]).String()
	fileType := "document"
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		fileType = "image"
	case strings.HasPrefix(mimeType, "video/"):
		fileType = "video"
	case strings.HasPrefix(mimeType, "audio/"):
		fileType = "audio"
	}

	return FileInfo{
		Type:     fileType,
		MIMEType: mimeType,
		FileSize: size,
	}, nil
}

func GetVideoInfo(inputPath string) (*VideoInfo, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,display_aspect_ratio",
		"-show_entries", "format=duration",
		"-of", "json",
		inputPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ffprobe: %w", err)
	}

	var probeOutput struct {
		Streams []struct {
			Width              int    `json:"width"`
			Height             int    `json:"height"`
			DisplayAspectRatio string `json:"display_aspect_ratio"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &probeOutput); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	if len(probeOutput.Streams) == 0 {
		return nil, fmt.Errorf("no video stream found")
	}

	width := probeOutput.Streams[0].Width
	height := probeOutput.Streams[0].Height

	aspect := float64(width) / float64(height)
	closestName := closestAspectRatio(aspect)

	duration, err := strconv.ParseFloat(probeOutput.Format.Duration, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid duration: %w", err)
	}

	return &VideoInfo{
		Width:       width,
		Height:      height,
		AspectRatio: closestName,
		Duration:    duration,
	}, nil
}

func GetImageInfo(path string) (*ImageInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}
	defer file.Close()

	img, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	aspect := float64(img.Width) / float64(img.Height)
	closestName := closestAspectRatio(aspect)

	return &ImageInfo{
		Width:       img.Width,
		Height:      img.Height,
		AspectRatio: closestName,
	}, nil
}

func closestAspectRatio(aspect float64) string {
	var closestName string
	minDiff := math.MaxFloat64
	for name, ratio := range aspectRatios {
		diff := math.Abs(aspect - ratio)
		if diff < minDiff {
			minDiff = diff
			closestName = name
		}
	}
	return closestName
}

// GetAudioDuration returns the duration of the audio file in seconds
func GetAudioDuration(filePath string) (float64, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run ffprobe: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

func OnUpload(fn func(media *Media) error) {
	mediaUploadedCallbacks = append(mediaUploadedCallbacks, fn)
}

// MoveFile moves a file from src to dst.
// It tries os.Rename first, and falls back to copy+delete if needed (e.g., on cross-filesystem error).
func MoveFile(src, dst string) error {
	// Ensure destination directory exists
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Try fast rename
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Check for cross-device error (EXDEV)
	linkErr, ok := err.(*os.LinkError)
	if !ok || linkErr.Err != syscall.EXDEV {
		// Return immediately if it's not a cross-device move issue
		return fmt.Errorf("failed to rename: %w", err)
	}

	// Fallback: copy + delete
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Preserve permissions
	if stat, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, stat.Mode())
	}

	// Delete the original
	if err = os.Remove(src); err != nil {
		return fmt.Errorf("failed to remove source file: %w", err)
	}

	return nil
}

// ExtractMediaMetadata extracts media metadata using ffprobe
func ExtractMediaMetadata(media *Media) []MetaData {
	var metadata []MetaData
	var err error
	evo.Dump(media)
	switch media.Type {
	case "video":
		metadata, err = ExtractVideoMetadata(media)
	case "image":
		metadata, err = ExtractImageExif(media)
	case "audio":
		metadata, err = ExtractAudioMetadata(media)
	default:
		return nil
	}
	if err != nil {
		log.Printf("Error extracting metadata for %s: %v", media.Path, err)
		return nil
	}
	return metadata
}
