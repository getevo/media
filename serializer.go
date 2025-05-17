package media

import "encoding/xml"

// InitiateMultipartUploadResult represents the XML structure.
type InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

func (v InitiateMultipartUploadResult) String() string {
	output, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}

	return xml.Header + string(output)
}

// CompleteMultipartUpload represents the XML structure to parse.
type CompleteMultipartUpload struct {
	XMLName xml.Name `xml:"CompleteMultipartUpload"`
	Parts   []Part   `xml:"Part"`
}

// Part represents each individual part in the multipart upload.
type Part struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

// FileInfo holds the detected file type and MIME type
type FileInfo struct {
	Type     string // one of: image, video, audio, document
	MIMEType string
}

type VideoInfo struct {
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	AspectRatio string  `json:"aspect_ratio"`
	Duration    float64 `json:"duration"` // seconds
}

// Famous aspect ratios with their float equivalents
var aspectRatios = map[string]float64{
	"16:9": 16.0 / 9.0,
	"4:3":  4.0 / 3.0,
	"3:2":  3.0 / 2.0,
	"1:1":  1.0,
	"21:9": 21.0 / 9.0,
	"5:4":  5.0 / 4.0,
	"2:1":  2.0,
}

type ImageInfo struct {
	Width       int
	Height      int
	AspectRatio string
}
