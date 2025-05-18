package media

import (
	"bytes"
	"fmt"
	"github.com/dhowden/tag"
	"github.com/getevo/evo/v2/lib/db"
	"github.com/getevo/evo/v2/lib/json"
	"github.com/rwcarlsen/goexif/exif"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ExtractImageExif(media *Media) ([]MetaData, error) {
	var metadata []MetaData

	absPath, err := filepath.Abs(filepath.Join(LocalUploadDir, media.Path))
	if err != nil {
		return nil, fmt.Errorf("absolute path error: %w", err)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open image: %w", err)
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode EXIF: %w", err)
	}

	// Common EXIF tags to extract
	tags := []string{
		"Make", "Model", "Software", "LensModel",
		"DateTime", "DateTimeOriginal", "SubSecTimeOriginal",
		"ExposureTime", "FNumber", "ISOSpeedRatings",
		"ShutterSpeedValue", "ApertureValue", "FocalLength",
		"Orientation", "WhiteBalance", "Flash",
		"PixelXDimension", "PixelYDimension",
		"XResolution", "YResolution", "ResolutionUnit",
	}

	exifVals := make(map[string]string)

	for _, tag := range tags {
		if val, err := x.Get(exif.FieldName(tag)); err == nil {
			if valStr, err := val.StringVal(); err == nil {
				exifVals[tag] = valStr
				metadata = append(metadata, MetaData{
					MediaID: media.MediaID,
					Key:     strings.ToLower(tag),
					Value:   valStr,
				})
			}
		}
	}

	// Width, height
	widthStr := exifVals["PixelXDimension"]
	heightStr := exifVals["PixelYDimension"]

	if widthStr != "" && heightStr != "" {
		metadata = append(metadata, MetaData{
			MediaID: media.MediaID,
			Key:     "width",
			Value:   widthStr,
		})
		metadata = append(metadata, MetaData{
			MediaID: media.MediaID,
			Key:     "height",
			Value:   heightStr,
		})

		// Aspect ratio
		var width, height float64
		fmt.Sscanf(widthStr, "%f", &width)
		fmt.Sscanf(heightStr, "%f", &height)
		if width > 0 && height > 0 {
			aspect := width / height
			metadata = append(metadata, MetaData{
				MediaID: media.MediaID,
				Key:     "aspect_ratio",
				Value:   fmt.Sprintf("%.2f", aspect),
			})
		}
	}

	// DPI calculation
	dpiX := exifVals["XResolution"]
	dpiY := exifVals["YResolution"]
	resUnit := strings.ToLower(exifVals["ResolutionUnit"]) // 2 = inches, 3 = cm

	if dpiX != "" && resUnit == "2" {
		metadata = append(metadata, MetaData{
			MediaID: media.MediaID,
			Key:     "dpi_x",
			Value:   dpiX,
		})
	}
	if dpiY != "" && resUnit == "2" {
		metadata = append(metadata, MetaData{
			MediaID: media.MediaID,
			Key:     "dpi_y",
			Value:   dpiY,
		})
	}

	// GPS
	if lat, long, err := x.LatLong(); err == nil {
		metadata = append(metadata, MetaData{
			MediaID: media.MediaID,
			Key:     "latitude",
			Value:   fmt.Sprintf("%f", lat),
		})
		metadata = append(metadata, MetaData{
			MediaID: media.MediaID,
			Key:     "longitude",
			Value:   fmt.Sprintf("%f", long),
		})
	}

	return metadata, nil
}
func ExtractAudioMetadata(media *Media) ([]MetaData, error) {
	var metadata []MetaData
	absPath, err := filepath.Abs(filepath.Join(LocalUploadDir, media.Path))
	if err != nil {
		return nil, fmt.Errorf("absolute path error: %w", err)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read tags: %w", err)
	}

	// Standard fields
	if m.Title() != "" {
		metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "title", Value: m.Title()})
	}
	if m.Artist() != "" {
		metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "artist", Value: m.Artist()})
	}
	if m.Album() != "" {
		metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "album", Value: m.Album()})
	}
	if m.Genre() != "" {
		metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "genre", Value: m.Genre()})
	}
	if m.Year() != 0 {
		metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "year", Value: fmt.Sprintf("%d", m.Year())})
	}
	if m.Composer() != "" {
		metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "composer", Value: m.Composer()})
	}

	// Save embedded picture (cover art)
	picture := m.Picture()
	if picture != nil && len(picture.Data) > 0 {
		dir := filepath.Dir(absPath)
		baseName := strings.TrimSuffix(filepath.Base(media.Path), filepath.Ext(media.Path))
		thumbName := fmt.Sprintf("%s_thumb.%s", baseName, picture.Ext)
		thumbPath := filepath.Join(dir, thumbName)

		if err := os.WriteFile(thumbPath, picture.Data, 0644); err != nil {
			return metadata, fmt.Errorf("failed to save thumbnail: %w", err)
		}

		media.Thumbnail = filepath.Join(filepath.Dir(media.Path), thumbName)
		db.Save(media)
	}

	return metadata, nil
}

func ExtractVideoMetadata(media *Media) ([]MetaData, error) {
	var metadata []MetaData

	absPath, err := filepath.Abs(filepath.Join(LocalUploadDir, media.Path))
	if err != nil {
		return nil, fmt.Errorf("absolute path error: %w", err)
	}
	media.Path = absPath

	if _, err := os.Stat(media.Path); err != nil {
		return nil, fmt.Errorf("file does not exist: %w", err)
	}

	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		media.Path,
	)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe error: %v, stderr: %s", err, stderr.String())
	}

	var result struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			CodecName  string `json:"codec_name"`
			RFrameRate string `json:"r_frame_rate"`
			Channels   int    `json:"channels"`
			Tags       struct {
				Language string `json:"language"`
			} `json:"tags"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe JSON: %w", err)
	}

	// Duration
	if result.Format.Duration != "" {
		metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "duration", Value: result.Format.Duration})
	}

	audioLangs := make(map[string]struct{})
	subtitleLangs := make(map[string]struct{})
	audioChannels := 0

	for _, stream := range result.Streams {
		switch stream.CodecType {
		case "video":
			if stream.Width > 0 && stream.Height > 0 {
				res := fmt.Sprintf("%dx%d", stream.Width, stream.Height)
				metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "resolution", Value: res})

				aspectRatio := fmt.Sprintf("%.2f", float64(stream.Width)/float64(stream.Height))
				metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "aspect_ratio", Value: aspectRatio})
			}
			if stream.CodecName != "" {
				metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "codec", Value: stream.CodecName})
			}
			if stream.RFrameRate != "" {
				metadata = append(metadata, MetaData{MediaID: media.MediaID, Key: "frame_rate", Value: stream.RFrameRate})
			}

		case "audio":
			lang := stream.Tags.Language
			if lang == "" {
				lang = "und" // undefined
			}
			audioLangs[lang] = struct{}{}
			audioChannels += stream.Channels

		case "subtitle":
			lang := stream.Tags.Language
			if lang == "" {
				lang = "und"
			}
			subtitleLangs[lang] = struct{}{}
		}
	}

	// Convert audio language map to list
	var audioLangList []string
	for lang := range audioLangs {
		audioLangList = append(audioLangList, lang)
	}
	metadata = append(metadata, MetaData{
		MediaID: media.MediaID,
		Key:     "audio_languages",
		Value:   strings.Join(audioLangList, ","),
	})

	// Total audio channels
	metadata = append(metadata, MetaData{
		MediaID: media.MediaID,
		Key:     "audio_channels",
		Value:   fmt.Sprintf("%d", audioChannels),
	})

	// Convert subtitle language map to list
	var subtitleLangList []string
	for lang := range subtitleLangs {
		subtitleLangList = append(subtitleLangList, lang)
	}
	metadata = append(metadata, MetaData{
		MediaID: media.MediaID,
		Key:     "subtitle_languages",
		Value:   strings.Join(subtitleLangList, ","),
	})

	return metadata, nil
}
