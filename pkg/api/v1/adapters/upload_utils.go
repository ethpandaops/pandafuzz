package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
)

var errUploadTooLarge = errors.New("upload too large")

func streamToTempFile(reader io.Reader, maxSize int64, computeHash bool) (string, int64, string, error) {
	tempFile, err := os.CreateTemp("", "pandafuzz-upload-*")
	if err != nil {
		return "", 0, "", err
	}
	tempPath := tempFile.Name()

	cleanup := func() {
		tempFile.Close()
		os.Remove(tempPath)
	}

	var hashWriter hash.Hash
	if computeHash {
		hashWriter = sha256.New()
	}

	writer := io.Writer(tempFile)
	if hashWriter != nil {
		writer = io.MultiWriter(tempFile, hashWriter)
	}

	source := reader
	if maxSize > 0 {
		source = io.LimitReader(reader, maxSize+1)
	}

	written, err := io.Copy(writer, source)
	if err != nil {
		cleanup()
		return "", 0, "", err
	}
	if maxSize > 0 && written > maxSize {
		cleanup()
		return "", 0, "", errUploadTooLarge
	}

	if err := tempFile.Close(); err != nil {
		cleanup()
		return "", 0, "", err
	}

	hash := ""
	if hashWriter != nil {
		hash = hex.EncodeToString(hashWriter.Sum(nil))
	}

	return tempPath, written, hash, nil
}
