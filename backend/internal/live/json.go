package live

import (
	"encoding/json"
	"strconv"
)

func decodeJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func itoa(n int) string { return strconv.Itoa(n) }
