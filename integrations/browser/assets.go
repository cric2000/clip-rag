package browser

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
)

//go:embed identity.json
var identity []byte

const HostName = "io.github.cliprag.browser"

func ExtensionID() string {
	var value struct{ ID, Key string }
	if err := json.Unmarshal(identity, &value); err != nil {
		panic(err)
	}
	key, err := base64.StdEncoding.DecodeString(value.Key)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(key)
	var result []byte
	for _, n := range digest[:16] {
		result = append(result, 'a'+n/16, 'a'+n%16)
	}
	if string(result) != value.ID {
		panic("browser extension identity mismatch")
	}
	return value.ID
}
