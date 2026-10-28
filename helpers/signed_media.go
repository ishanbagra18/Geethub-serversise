package helpers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

func getMediaSigningSecret() []byte {
	secret := os.Getenv("MEDIA_SIGNING_SECRET")
	if secret == "" {
		secret = os.Getenv("SECRET_KEY")
	}
	if secret == "" {
		secret = "geethub_default_secure_media_signing_token_2026_xyz"
	}
	return []byte(secret)
}

// GenerateSignedMediaURL creates a time-limited HMAC-SHA256 signature for streaming a song
func GenerateSignedMediaURL(songID string, userID string, duration time.Duration) (string, int64) {
	expiresAt := time.Now().Add(duration).Unix()
	expiresStr := strconv.FormatInt(expiresAt, 10)

	payload := fmt.Sprintf("%s:%s:%s", songID, userID, expiresStr)
	mac := hmac.New(sha256.New, getMediaSigningSecret())
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	queryURL := fmt.Sprintf("/music/stream/%s?expires=%s&uid=%s&sig=%s", songID, expiresStr, userID, signature)
	return queryURL, expiresAt
}

// VerifyMediaSignature verifies the HMAC signature and expiration timestamp
func VerifyMediaSignature(songID string, userID string, expiresStr string, signature string) error {
	expiresAt, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return errors.New("invalid expiration timestamp")
	}

	if time.Now().Unix() > expiresAt {
		return errors.New("media URL has expired")
	}

	payload := fmt.Sprintf("%s:%s:%s", songID, userID, expiresStr)
	mac := hmac.New(sha256.New, getMediaSigningSecret())
	mac.Write([]byte(payload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return errors.New("invalid media signature")
	}

	return nil
}
