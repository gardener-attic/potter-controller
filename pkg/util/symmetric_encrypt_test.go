package util

import (
	"encoding/hex"
	"testing"

	"github.com/arschles/assert"
)

func TestHash256(t *testing.T) {
	hash, err := Hash256("hello")
	assert.Nil(t, err, "hash error")
	assert.Equal(t, len(hash), 32, "hash length")
	assert.Equal(t, hex.EncodeToString(hash), "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", "hash")

	hash, err = Hash256("")
	assert.Nil(t, err, "hash error")
	assert.Equal(t, len(hash), 32, "hash length of empty phrase")
	assert.Equal(t, hex.EncodeToString(hash), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "hash of empty phrase")
}

func TestComputeSecretDeletionToken(t *testing.T) {
	hash, err := Hash256("test")
	assert.Nil(t, err, "error")

	token, err := ComputeSecretDeletionToken(hash, "testSecretName1")
	assert.Nil(t, err, "error")
	assert.Equal(t, hex.EncodeToString(token), "01dd5581323ee91abd891b2906e449ba84487295166109f1e717d2c1f29c2b03", "token 1")

	token2, err := ComputeSecretDeletionToken(hash, "testSecretName2")
	assert.Nil(t, err, "error")
	assert.Equal(t, hex.EncodeToString(token2), "ed853e20247a81e052c84f6f29414ea618fd1beafcc44457e8747951444b8276", "token 2")

	hash2, err := Hash256("test2")
	assert.Nil(t, err, "error")

	token3, err := ComputeSecretDeletionToken(hash2, "testSecretName1")
	assert.Nil(t, err, "error")
	assert.Equal(t, hex.EncodeToString(token3), "ff9f8e46a105854f8dea0d21ba1eac62542bba0d9f7621cb1f0e668a8d9e94ea", "token 3")

	token4, err := ComputeSecretDeletionToken(hash2, "testSecretName2")
	assert.Nil(t, err, "error")
	assert.Equal(t, hex.EncodeToString(token4), "4532d614e45c6bdcb1acc980dd14f8cbd992efdd49bd2f04047eb0899aa7095b", "token 4")

	token5, err := ComputeSecretDeletionToken(hash2, "testSecretName2")
	assert.Nil(t, err, "error")
	assert.Equal(t, hex.EncodeToString(token5), "4532d614e45c6bdcb1acc980dd14f8cbd992efdd49bd2f04047eb0899aa7095b", "token 5")
}
