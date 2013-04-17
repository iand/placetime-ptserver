package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io"
)

// Some helper functions

func RandomString(length int) (string, error) {
	randomBytes := make([]byte, length)
	_, err := io.ReadFull(rand.Reader, randomBytes)
	if err != nil {
		return "", err
	}

	nonceBuffer := &bytes.Buffer{}
	encoder := base64.NewEncoder(base64.StdEncoding, nonceBuffer)
	defer encoder.Close()
	encoder.Write(randomBytes)

	return nonceBuffer.String(), nil
}
