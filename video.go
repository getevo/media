package media

import (
	"bytes"
	"fmt"
	"github.com/getevo/evo/v2/lib/text"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

func CreateVideoPreview(media *Media) error {
	absInput, err := getPath(filepath.Join(LocalUploadDir, media.Path))
	if err != nil {
		return fmt.Errorf("absolute input path error: %w", err)
	}
	absOutput, err := getPath(filepath.Join(LocalUploadDir, filepath.Dir(media.Path), "preview.mp4"))
	if err != nil {
		return fmt.Errorf("absolute output path error: %w", err)
	}

	// Step 1: Get duration
	var duration = float64(media.Duration)

	// Create temp directory
	tmpDir := filepath.Join(filepath.Dir(absOutput), "tmp_preview")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if duration < 30 {
		// Simple case: first 10s
		return ffmpegExtract(absInput, absOutput, 0, 10)
	}

	// Complex case: Split and process
	partDuration := duration / 4.0
	var wg sync.WaitGroup
	var errs = make([]error, 4)

	for i := 0; i < 4; i++ {
		start := partDuration * float64(i)
		out := filepath.Join(tmpDir, fmt.Sprintf("part%d.mp4", i))
		wg.Add(1)
		go func(i int, start float64, out string) {
			defer wg.Done()
			errs[i] = ffmpegExtract(absInput, out, start, 2.5)
		}(i, start, out)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return fmt.Errorf("failed to extract preview parts: %w", err)
		}
	}

	// Create concat list
	var random = text.Random(5)
	concatFile := filepath.Join(tmpDir, "concat-"+random+".txt")
	var concatList strings.Builder
	for i := 0; i < 4; i++ {
		concatList.WriteString(fmt.Sprintf("file '%s'\n", filepath.Join(tmpDir, fmt.Sprintf("part%d.mp4", i))))
	}
	if err := os.WriteFile(concatFile, []byte(concatList.String()), 0644); err != nil {
		return fmt.Errorf("failed to write concat file: %w", err)
	}

	// Final concat
	cmd := exec.Command("ffmpeg",
		"-y", "-f", "concat", "-safe", "0",
		"-i", concatFile,
		"-c", "copy",
		filepath.Join(tmpDir, "combined.mp4"),
	)
	if err := runCmd(cmd); err != nil {
		return fmt.Errorf("failed to concat: %w", err)
	}
	defer os.Remove(concatFile)
	// Resize + remove audio from combined
	err = ffmpegFinalize(filepath.Join(tmpDir, "combined.mp4"), absOutput)
	if err != nil {
		return fmt.Errorf("failed to finalize combined: %w", err)
	}
	media.Preview = filepath.Join(filepath.Dir(media.Path), "preview.mp4")
	return nil
}

func ffmpegExtract(input, output string, start, duration float64) error {
	cmd := exec.Command("ffmpeg",
		"-y",
		"-ss", fmt.Sprintf("%.2f", start),
		"-t", fmt.Sprintf("%.2f", duration),
		"-i", input,
		"-an",                 // remove audio
		"-vf", "scale=-2:480", // 480p
		"-c:v", "libx264", // h264
		"-preset", "fast",
		output,
	)
	return runCmd(cmd)
}

func ffmpegFinalize(input, output string) error {
	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", input,
		"-an",                 // remove audio
		"-vf", "scale=-2:480", // 480p
		"-c:v", "libx264",
		"-preset", "fast",
		output,
	)
	return runCmd(cmd)
}

func runCmd(cmd *exec.Cmd) error {
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("cmd failed: %v - stderr: %s", err, stderr.String())
	}
	return nil
}

// GenerateVideoThumbnail generates a 720p JPG thumbnail from the midpoint of the video.
// Returns the absolute thumbnail path.
func GenerateVideoThumbnail(media *Media) error {
	fmt.Println(media.Path)
	absInput, err := getPath(filepath.Join(LocalUploadDir, media.Path))
	if err != nil {
		return fmt.Errorf("absolute input path error: %w", err)
	}
	absOutput, err := getPath(filepath.Join(LocalUploadDir, filepath.Dir(media.Path), "preview.jpg"))
	if err != nil {
		return fmt.Errorf("absolute output path error: %w", err)
	}
	fmt.Println("Generating thumbnail for:", absInput)
	fmt.Println("Output thumbnail:", absOutput)
	// Grab frame at middle of video
	midpoint := media.Duration / 2.0

	cmd := exec.Command("ffmpeg",
		"-y",
		"-ss", fmt.Sprintf("%.2f", midpoint),
		"-i", absInput,
		"-vframes", "1",
		"-q:v", "2", // High quality JPEG
		"-vf", "scale=-1:720", // scale to 720p height, keep aspect ratio
		absOutput,
	)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("thumbnail generation failed: %w\n%s", err, stderr.String())
	}
	media.Thumbnail = filepath.Join(filepath.Dir(media.Path), "preview.jpg")
	return nil
}

func getPath(mediaPath string) (string, error) {
	if filepath.IsAbs(mediaPath) {
		return mediaPath, nil
	}
	return filepath.Abs(mediaPath)
}
