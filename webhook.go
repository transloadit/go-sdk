package transloadit

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

// AssemblyNotificationPayload is the payload Transloadit submits to an
// assembly's notify_url once it finishes. Its shape matches AssemblyInfo, the
// result of GetAssembly. Use ParseAssemblyNotification to verify and decode
// an incoming notification request.
// See https://transloadit.com/docs/topics/assembly-instructions/#notifications
type AssemblyNotificationPayload struct {
	AssemblyInfo
}

// ParseAssemblyNotification verifies the signature of an incoming Assembly
// Notification request and decodes its payload. authSecret must be the
// Client's Config.AuthSecret.
//
// Transloadit sends notifications as a form-urlencoded POST request with the
// JSON payload in a "transloadit" field and a hex-encoded HMAC-SHA1
// signature, computed over that field using authSecret, in a "signature"
// field. A successful HTTP response only acknowledges delivery; check
// AssemblyNotificationPayload.Ok and .Error for the assembly's outcome.
func ParseAssemblyNotification(r *http.Request, authSecret string) (*AssemblyNotificationPayload, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("parse assembly notification: %s", err)
	}

	payload := r.FormValue("transloadit")
	signature := r.FormValue("signature")
	if payload == "" || signature == "" {
		return nil, fmt.Errorf("parse assembly notification: missing transloadit or signature field")
	}

	hash := hmac.New(sha1.New, []byte(authSecret))
	hash.Write([]byte(payload))
	expected := hex.EncodeToString(hash.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, fmt.Errorf("parse assembly notification: signature mismatch")
	}

	var result AssemblyNotificationPayload
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return nil, fmt.Errorf("parse assembly notification: %s", err)
	}

	return &result, nil
}
