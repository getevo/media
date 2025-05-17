package media

type StorageIFace interface {
	Initialize() error
	Put(src string, dst string) error
	Delete(dst string) error
	Get(dst string, src string) error
	List(dir string, recursive bool) ([]string, error)
}
