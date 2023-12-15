package o

import sneaky "os"

func m() {
	sneaky.ReadFile("any") // want `os.ReadFile\("any"\)`
}
