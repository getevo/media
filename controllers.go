package media

import (
	"errors"
	"fmt"
	"github.com/getevo/evo/v2"
	"github.com/getevo/evo/v2/lib/db"
	"github.com/getevo/evo/v2/lib/gpath"
	"github.com/getevo/evo/v2/lib/log"
	"github.com/getevo/evo/v2/lib/outcome"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"
)

type Controller struct{}
type UploadInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	FileName    string `json:"filename"`
	IsBase64    bool   `json:"base64"`
	Content     string `json:"content"`
}

func (c Controller) BasicUploadHandler(request *evo.Request) any {
	var file *multipart.FileHeader
	var input UploadInput
	var err = request.BodyParser(&input)
	if err != nil {
		return err
	}
	var filename = input.FileName
	if input.IsBase64 {
		var content = input.Content
		if content == "" {
			return errors.New("invalid base64 content")
		}
		if filename == "" {
			return errors.New("filename is required")
		}
		file, err = CreateMultipartFileHeader(content, filename)
	} else {
		file, err = request.FormFile("file")
		filename = file.Filename
	}

	if err != nil {
		return err
	}
	if filename != "" {
		filename = NormalizeFileName(filename)
	}

	fileType, err := DetectFileType(file)
	if err != nil {
		return err
	}

	var media = Media{
		Title:       request.BodyValue("title").String(),
		Filename:    filename,
		Description: request.BodyValue("description").String(),
		FileSize:    file.Size,
		Type:        fileType.Type,
		Mimetype:    fileType.MIMEType,
	}

	if media.Title == "" {
		media.Title = media.Filename
	}

	if !slices.Contains([]string{"video", "image", "audio", "document"}, media.Type) {
		return errors.New("media type is missing")
	}

	media.Status = READY
	media.Path = fmt.Sprint(time.Now().Unix())
	var destination = path.Join(LocalUploadDir, media.Path)
	err = gpath.MakePath(destination)

	if err != nil {
		log.Error(err)
		return err
	}
	if err = request.SaveFile(file, path.Join(destination, media.Filename)); err != nil {
		log.Error(err)
		return err
	}

	switch fileType.Type {
	case "video":
		var info, err = GetVideoInfo(path.Join(destination, media.Filename))
		if err != nil {
			log.Error(err)
			return err
		}
		media.Duration = int64(info.Duration)
		media.ScreenSize = fmt.Sprintf("%dx%d", info.Width, info.Height)
		media.AspectRatio = info.AspectRatio
	case "image":
		var info, err = GetImageInfo(path.Join(destination, media.Filename))
		if err != nil {
			log.Error(err)
			return err
		}
		media.ScreenSize = fmt.Sprintf("%dx%d", info.Width, info.Height)
		media.AspectRatio = info.AspectRatio
	case "audio":
		var duration, err = GetAudioDuration(path.Join(destination, media.Filename))
		if err != nil {
			log.Error(err)
			return err
		}
		media.Duration = int64(duration)
	}

	media.Path += "/" + media.Filename
	media.FileSize = file.Size

	if !request.BodyValue("skip_save").Bool() {
		if err = db.Save(&media).Error; err != nil {
			return err
		}
	}
	for _, callback := range mediaUploadedCallbacks {
		err = callback(&media)
		if err != nil {
			return err
		}
	}

	return media
}

func (c Controller) MultipartUploadHandler(request *evo.Request) any {

	var key = request.Param("*").String()
	key = NormalizeFileName(key)
	var uploadID = request.Query("uploadId").String()

	// Upload completed
	if uploadID != "" {
		var data CompleteMultipartUpload
		err := request.BodyParser(&data)
		if err != nil {
			return err
		}

		// upload to S3
		var media Media
		if db.Where("external_id =?", uploadID).Find(&media).RowsAffected == 0 {
			return errors.New("invalid upload ID")
		}

		err, file := c.AssembleUpload(key, uploadID, data)
		if err != nil {
			return err
		}

		fileType, err := DetectFileType(file)
		if err != nil {
			return err
		}
		media.Mimetype = fileType.MIMEType
		media.Type = fileType.Type
		media.Status = PROCESSING
		media.Path = filepath.Join(uploadID, key)
		media.FileSize = fileType.FileSize
		switch fileType.Type {
		case "video":
			var info, err = GetVideoInfo(file)
			if err != nil {
				log.Error(err)
				return err
			}
			media.Duration = int64(info.Duration)
			media.ScreenSize = fmt.Sprintf("%dx%d", info.Width, info.Height)
			media.AspectRatio = info.AspectRatio
		case "image":
			var info, err = GetImageInfo(file)
			if err != nil {
				log.Error(err)
				return err
			}
			media.ScreenSize = fmt.Sprintf("%dx%d", info.Width, info.Height)
			media.AspectRatio = info.AspectRatio
		case "audio":
			var duration, err = GetAudioDuration(file)
			if err != nil {
				log.Error(err)
				return err
			}
			media.Duration = int64(duration)
		}

		err = MoveFile(file, filepath.Join(LocalUploadDir, media.Path))
		if err != nil {
			return err
		}
		media.Status = READY
		db.Save(&media)
		for _, callback := range mediaUploadedCallbacks {
			err = callback(&media)
			if err != nil {
				return err
			}
		}
		return outcome.Response{
			StatusCode: 200,
		}
	}

	uploadID = fmt.Sprint(time.Now().UnixNano())
	var media = Media{
		Filename:    key,
		Title:       request.Header("X-File-Title"),
		Description: request.Header("X-File-Description"),
		Type:        request.Header("X-File-Type"),
		Path:        uploadID + "/" + key,
		Status:      UPLOADING,
		ExternalID:  uploadID,
	}
	if err := db.Save(&media).Error; err != nil {
		return err
	}
	var upload = InitiateMultipartUploadResult{
		Bucket:   "upload",
		Key:      key,
		UploadID: uploadID,
	}
	_ = gpath.MakePath(filepath.Join(TemporaryDir, upload.Key, uploadID))

	return outcome.Response{
		StatusCode: 200,
		Data:       upload.String(),
		Headers: map[string]string{
			"Content-Type": "text/xml",
		},
	}

}

func (c Controller) MultipartCleanUploadHandler(request *evo.Request) any {
	var key = request.Param("*").String()
	key = NormalizeFileName(key)
	_ = gpath.Remove(filepath.Join(TemporaryDir, key))
	return outcome.Response{
		StatusCode: 204,
	}
}

var counter = 0

func (c Controller) MultipartUploadChunkHandler(request *evo.Request) any {
	var key = request.Param("*").String()
	key = NormalizeFileName(key)
	var uploadID = request.Query("uploadId").String()
	var partNumber = request.Query("partNumber").Int()

	var p = filepath.Join(TemporaryDir, key, uploadID, fmt.Sprintf("%d", partNumber))
	err := gpath.Write(p, request.Context.Body())
	if err != nil {
		return err
	}
	counter++
	return outcome.Response{
		StatusCode: 200,
		Data:       "",
		Headers: map[string]string{
			"Etag": fmt.Sprintf("%d%d", time.Now().UnixNano(), counter),
		},
	}
}

func (c Controller) AssembleUpload(key string, uploadID string, data CompleteMultipartUpload) (error, string) {
	var p = filepath.Join(TemporaryDir, key, uploadID)
	_ = gpath.MakePath(filepath.Join(TemporaryDir, "assembled"))
	var filePath = filepath.Join(TemporaryDir, "assembled", key)
	out, err := os.Create(filePath)
	if err != nil {
		return err, filePath
	}
	defer out.Close()

	for i := 1; i <= len(data.Parts); i++ {
		partPath := p + "/" + fmt.Sprintf("%d", i)
		in, err := os.Open(partPath)
		if err != nil {
			return err, filePath
		}
		defer in.Close()

		_, err = io.Copy(out, in)
		if err != nil {
			return err, filePath
		}
	}

	go func() {
		time.Sleep(1 * time.Second)
		_ = gpath.Remove(filepath.Dir(p))
	}()

	return nil, filePath
}
