package p

import (
	sneaky "encoding/hex"
	"path/filepath"
)

func pkg() {
	filepath.Join("", "/")  // want `filepath.Join\("", "/"\)`
	filepath.Dir("any")     // want `filepath.Dir\("any"\)`
	filepath.Dir("any")     // want `filepath.Dir\("any"\)`
	sneaky.DecodeString("") // want `encoding/hex.DecodeString\(""\)`
}
