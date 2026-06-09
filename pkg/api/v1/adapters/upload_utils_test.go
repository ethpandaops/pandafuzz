package adapters

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamToTempFileWithHash(t *testing.T) {
	payload := []byte("pandafuzz-upload-test")

	path, size, hash, err := streamToTempFile(bytes.NewReader(payload), int64(len(payload)), true)
	require.NoError(t, err)
	require.NotEmpty(t, path)
	require.Equal(t, int64(len(payload)), size)

	expectedHash := sha256.Sum256(payload)
	require.Equal(t, hex.EncodeToString(expectedHash[:]), hash)

	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, payload, onDisk)

	require.NoError(t, os.Remove(path))
}

func TestStreamToTempFileSizeLimit(t *testing.T) {
	payload := []byte("size-limit-test")

	path, size, hash, err := streamToTempFile(bytes.NewReader(payload), 4, true)
	require.ErrorIs(t, err, errUploadTooLarge)
	require.Empty(t, path)
	require.Zero(t, size)
	require.Empty(t, hash)
}

func TestStreamToTempFileNoHash(t *testing.T) {
	payload := []byte("no-hash-test")

	path, size, hash, err := streamToTempFile(bytes.NewReader(payload), int64(len(payload)), false)
	require.NoError(t, err)
	require.NotEmpty(t, path)
	require.Equal(t, int64(len(payload)), size)
	require.Empty(t, hash)

	require.NoError(t, os.Remove(path))
}
