package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
)

func Example() {
	text := []byte("My name is Astaxie")
	key := []byte("the-key-has-to-be-32-bytes-long!")

	ciphertext, err := Encrypt(text, key)
	if err != nil {
		// TODO: Properly handle error
		log.Fatal(err)
	}
	fmt.Printf("%s => %x\n", text, ciphertext)

	plaintext, err := Decrypt(ciphertext, key)
	if err != nil {
		// TODO: Properly handle error
		log.Fatal(err)
	}
	fmt.Printf("%x => %s\n", ciphertext, plaintext)
}

func Encrypt(plaintext, key []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func Decrypt(ciphertext, key []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// Hash256 computes the SHA256 hash of the given string. The resulting []byte has length 32.
func Hash256(phrase string) ([]byte, error) {
	hasher := sha256.New()

	_, err := hasher.Write([]byte(phrase))
	if err != nil {
		return nil, err
	}

	hash := hasher.Sum(nil)
	return hash, nil
}

func HashObject256(obj interface{}) (string, error) {
	objBytes, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()

	_, err = hasher.Write(objBytes)
	if err != nil {
		return "", err
	}

	hash := hasher.Sum(nil)

	return base64.StdEncoding.EncodeToString(hash), nil
}

func ComputeSecretDeletionToken(secretDeletionKey []byte, secretName string) ([]byte, error) {
	hasher := sha256.New()

	_, err := hasher.Write(secretDeletionKey)
	if err != nil {
		return nil, err
	}

	_, err = hasher.Write([]byte(secretName))
	if err != nil {
		return nil, err
	}

	hash := hasher.Sum(nil)
	return hash, nil
}
