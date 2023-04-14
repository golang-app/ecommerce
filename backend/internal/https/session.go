package https

import (
	"crypto/rand"
	"encoding/base64"
)

// Generate a random session ID
func SessionID() string {
	// Create a byte slice to store the random bytes
	b := make([]byte, 32)

	// Read 32 random bytes from the crypto/rand package
	if _, err := rand.Read(b); err != nil {
		// If there was an error, panic with the error message
		panic(err)
	}

	// Encode the random bytes as a base64 string and return
	return base64.StdEncoding.EncodeToString(b)
}
