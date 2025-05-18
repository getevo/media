package media

import (
	"errors"
	"github.com/getevo/evo/v2"
	"github.com/getevo/evo/v2/lib/db"
	"github.com/getevo/evo/v2/lib/gpath"
	"github.com/getevo/evo/v2/lib/log"
	"github.com/getevo/evo/v2/lib/settings"
)

const (
	READY      = "ready"
	FAILED     = "failed"
	PROCESSING = "processing"
	UPLOADING  = "uploading"
)

var TemporaryDir = ""
var LocalUploadDir = ""
var mediaUploadedCallbacks []func(media *Media) error

type App struct{}

func (a App) Register() error {
	db.UseModel(Media{}, Collection{}, CollectionItems{}, MetaData{})
	/*	var err = db.SetupJoinTable(&Media{}, "Collections", &CollectionItems{})
		if err != nil {
			return err
		}*/
	// check ffmpeg installed
	if !IsFFMpegInstalled() {
		return errors.New("ffmpeg is not installed")
	}
	if !IsFFProbeInstalled() {
		return errors.New("ffprobe is not installed")
	}

	TemporaryDir = settings.Get("MEDIA.TEMPORARY_DIR").String()
	if TemporaryDir == "" {
		TemporaryDir = "./tmp"
	}

	LocalUploadDir = settings.Get("MEDIA.UPLOAD_DIR").String()
	if LocalUploadDir == "" {
		LocalUploadDir = "./uploads"
	}

	_ = gpath.MakePath(TemporaryDir)
	_ = gpath.MakePath(LocalUploadDir)

	OnUpload(func(media *Media) error {
		metadata, err := ExtractMediaMetadata(media)
		if err != nil {
			log.Error(err)
		} else {
			db.Save(metadata)
		}
		return nil
	})

	return nil
}

func (a App) Router() error {
	var controller Controller
	var admin = evo.Group("/admin/media")
	admin.Post("/upload", controller.BasicUploadHandler)
	admin.Post("/multipart/upload/*", controller.MultipartUploadHandler)
	admin.Delete("/multipart/upload/*", controller.MultipartCleanUploadHandler)
	admin.Put("/multipart/upload/*", controller.MultipartUploadChunkHandler)
	evo.Static("/upload", "./media/static")
	return nil
}

func (a App) WhenReady() error {
	return nil
}

func (a App) Name() string {
	return "media"
}
