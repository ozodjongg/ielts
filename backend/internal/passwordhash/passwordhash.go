package passwordhash

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	algorithm  = "pbkdf2_sha256"
	iterations = 600000
	saltBytes  = 16
	keyBytes   = 32
)

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	hLen := sha256.Size
	blocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func Hash(password string) (string, error) {
	if len(password) < 10 || len(password) > 128 {
		return "", errors.New("password must be 10-128 characters")
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2SHA256([]byte(password), salt, iterations, keyBytes)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", algorithm, iterations, enc.EncodeToString(salt), enc.EncodeToString(dk)), nil
}

func Verify(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != algorithm {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 100000 || iter > 2000000 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[2])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	want, err := enc.DecodeString(parts[3])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}
