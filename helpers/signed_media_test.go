package helpers

import (
	"net/url"
	"testing"
	"time"
)

func TestGenerateAndVerifyMediaSignature(t *testing.T) {
	songID := "song-test-123"
	userID := "user-alice-456"
	duration := 2 * time.Hour

	streamPath, expiresAt := GenerateSignedMediaURL(songID, userID, duration)
	if streamPath == "" || expiresAt <= time.Now().Unix() {
		t.Fatalf("expected valid stream path and future expiration, got %v, %v", streamPath, expiresAt)
	}

	u, err := url.Parse(streamPath)
	if err != nil {
		t.Fatalf("failed to parse generated URL: %v", err)
	}

	q := u.Query()
	expiresStr := q.Get("expires")
	uid := q.Get("uid")
	sig := q.Get("sig")

	if expiresStr == "" || uid != userID || sig == "" {
		t.Fatalf("missing or mismatched query params in generated URL: %v", q)
	}

	// 1. Valid verification
	if err := VerifyMediaSignature(songID, uid, expiresStr, sig); err != nil {
		t.Fatalf("expected signature to be valid, got error: %v", err)
	}

	// 2. Tampered signature
	if err := VerifyMediaSignature(songID, uid, expiresStr, sig+"tampered"); err == nil {
		t.Fatalf("expected tampered signature to fail verification")
	}

	// 3. Tampered song ID
	if err := VerifyMediaSignature("song-different-id", uid, expiresStr, sig); err == nil {
		t.Fatalf("expected mismatched songID to fail verification")
	}

	// 4. Tampered user ID
	if err := VerifyMediaSignature(songID, "user-bob-attacker", expiresStr, sig); err == nil {
		t.Fatalf("expected mismatched userID to fail verification")
	}

	// 5. Expired signature
	expiredPath, _ := GenerateSignedMediaURL(songID, userID, -10*time.Minute)
	expU, _ := url.Parse(expiredPath)
	expQ := expU.Query()
	if err := VerifyMediaSignature(songID, expQ.Get("uid"), expQ.Get("expires"), expQ.Get("sig")); err == nil {
		t.Fatalf("expected expired token to fail verification")
	}
}
