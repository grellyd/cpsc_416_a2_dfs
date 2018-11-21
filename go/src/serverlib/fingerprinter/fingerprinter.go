// Responsible for the generation of a collison free fingerprint.
package fingerprinter

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	ID_LENGTH = 2
)

// Hashmap of used Ids.
// defaults to false
// true if used
var existingPrints = map[string]bool{}

var characters = []rune("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func Generate() (print string, err error) {
	length := ID_LENGTH
	maxEntries := len(characters) ^ length
	for i := 0; i < maxEntries; i++ {
		print = generateRandomString(length)
		if !existingPrints[print] {
			existingPrints[print] = true
			return print, nil
		}
	}
	return "", fmt.Errorf("Maximum Entries Reached. Expand ID space.")
}

func generateRandomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]rune, length)
	for i := range b {
		b[i] = characters[rand.Intn(len(characters))]
	}
	return string(b)
}
