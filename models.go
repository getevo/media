package media

import (
	"github.com/getevo/evo/v2/lib/db/types"
	"github.com/getevo/restify"
)

type Media struct {
	MediaID        int64        `gorm:"column:media_id;primaryKey;autoIncrement" json:"media_id"`
	ExternalID     string       `gorm:"column:external_id;size:64;index" json:"external_id"`
	ExternalStatus string       `gorm:"column:state;size:64" json:"state"`
	Title          string       `gorm:"column:title" json:"title"`
	Filename       string       `gorm:"column:filename" json:"filename"`
	Path           string       `gorm:"column:path" json:"path"`
	Description    string       `gorm:"column:description" json:"description"`
	Thumbnail      string       `gorm:"column:thumbnail" json:"thumbnail"`
	Type           string       `gorm:"column:type;type:enum('image','audio','video','document')" json:"type"`
	Mimetype       string       `gorm:"column:mimetype" json:"mimetype"`
	Duration       int64        `gorm:"column:duration" json:"duration"`
	ScreenSize     string       `gorm:"column:screen_size" json:"screen_size"`
	AspectRatio    string       `gorm:"column:aspect_ratio;size:16" json:"aspect_ratio"`
	FileSize       int64        `gorm:"column:file_size" json:"file_size"`
	Status         string       `gorm:"column:status;type:enum('uploading','processing','ready','failed');index" json:"status"`
	Progress       float64      `gorm:"column:progress" json:"progress"`
	Error          string       `gorm:"column:error" json:"error"`
	MetaData       []MetaData   `gorm:"foreignKey:MediaID;references:MediaID" json:"metadata"`
	Collections    []Collection `gorm:"many2many:media_collection_items;joinTable:media_collection_items;foreignKey:CollectionID;references:CollectionID" json:""`
	types.CreatedAt
	types.UpdatedAt
	types.SoftDelete
	restify.API
}

func (Media) TableName() string {
	return "media"
}

type Collection struct {
	CollectionID int64  `gorm:"column:collection_id;primaryKey;autoIncrement" json:"collection_id"`
	Title        string `gorm:"column:title" json:"title"`
	Description  string `gorm:"column:description" json:"description"`
	types.CreatedAt
	types.UpdatedAt
	types.SoftDelete
	restify.API
}

func (Collection) TableName() string {
	return "media_collection"
}

type CollectionItems struct {
	CollectionID int64 `gorm:"column:collection_id;index;fk:media_collection" json:"collection_id"`
	MediaID      int64 `gorm:"column:media_id;index;fk:media_collection" json:"media_id"`
	VisualOrder  int   `gorm:"column:visual_order" json:"visual_order"`
	restify.API
}

func (CollectionItems) TableName() string {
	return "media_collection_items"
}

type MetaData struct {
	MetaDataID int64  `gorm:"column:meta_data_id;primaryKey;autoIncrement" json:"meta_data_id"`
	MediaID    int64  `gorm:"column:media_id;index;fk:media" json:"media_id"`
	Key        string `gorm:"column:key;size:64;index" json:"key"`
	Value      string `gorm:"column:value;index" json:"value"`
	restify.API
}

func (MetaData) TableName() string {
	return "media_metadata"
}
