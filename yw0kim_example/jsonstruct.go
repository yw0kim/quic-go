package jsonstruct

import (
	"os"
	"time"
)

/* FileInfo
Name() string       // base name of the file
Size() int64        // length in bytes for regular files; system-dependent for others
Mode() FileMode     // file mode bits
ModTime() time.Time // modification time
IsDir() bool        // abbreviation for Mode().IsDir()
Sys() interface{}
*/
type FileInfo struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	IsDir   bool
}

// FileInfos is slice of FileInfo
type FileInfos []FileInfo
