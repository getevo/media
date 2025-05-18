package media

type StorageIFace interface {
	Initialize() error
	PutFile(src string, dst string) error
	DeleteFile(dst string) error
	FetchFile(dst string, src string) error
	PutDir(src string, dst string) error
	DeleteDir(dst string) error
	FetchDir(dst string, src string) error
	List(dir string, recursive bool) ([]string, error)
}
