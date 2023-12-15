package http

import "net/http"

var header = http.Header{"key_1": []string{"value_1"}}

func set() {
	header.Set("key_2", "value_2") // want `net/http.Header.Set\("key_2", "value_2"\)`
	header.Add("key_3", "value_3") // want `net/http.Header.Add\("key_3", "value_3"\)`
}
