package s

import "strings"

var s1 = "literal_1"   // want `string: "literal_1"`
const s2 = "literal_2" // want `string: "literal_2"`

func anyFunc() {
	strings.Split("literal_3", "_") // want `string: "literal_3"` `string: "_"`
}
