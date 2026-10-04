package transloadit

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const examplePayload = `{"ok":"ASSEMBLY_COMPLETED","assembly_id":"76fe5df1c93a0a530f3e583805cf98b4","assembly_url":"https://api2.transloadit.com/assemblies/76fe5df1c93a0a530f3e583805cf98b4","region":"us-east-1","results":{"resize":[{"id":"f1","name":"lol_cat.jpg","basename":"lol_cat","ext":"jpg","size":1234,"mime":"image/jpeg","type":"image","field":"image","url":"https://example.com/lol_cat.jpg","ssl_url":"https://example.com/lol_cat.jpg"}]}}`

func signPayload(authSecret, payload string) string {
	hash := hmac.New(sha1.New, []byte(authSecret))
	hash.Write([]byte(payload))
	return hex.EncodeToString(hash.Sum(nil))
}

func newNotificationRequest(payload, signature string) *http.Request {
	form := url.Values{"transloadit": {payload}, "signature": {signature}}
	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestParseAssemblyNotification_VerifiesSignatureAndUnmarshals(t *testing.T) {
	const authSecret = "foo_secret"
	signature := signPayload(authSecret, examplePayload)

	req := newNotificationRequest(examplePayload, signature)

	notification, err := ParseAssemblyNotification(req, authSecret)
	if err != nil {
		t.Fatal(err)
	}

	if notification.Ok != "ASSEMBLY_COMPLETED" {
		t.Errorf("expected ok=ASSEMBLY_COMPLETED, got %q", notification.Ok)
	}
	if notification.AssemblyID != "76fe5df1c93a0a530f3e583805cf98b4" {
		t.Errorf("expected assembly_id to be set, got %q", notification.AssemblyID)
	}
	if notification.Region != "us-east-1" {
		t.Errorf("expected region=us-east-1, got %q", notification.Region)
	}
	if len(notification.Results["resize"]) != 1 || notification.Results["resize"][0].URL != "https://example.com/lol_cat.jpg" {
		t.Errorf("expected one resize result with the expected URL, got %#v", notification.Results["resize"])
	}
}

func TestParseAssemblyNotification_RejectsTamperedPayload(t *testing.T) {
	const authSecret = "foo_secret"
	signature := signPayload(authSecret, examplePayload)

	tampered := strings.Replace(examplePayload, "ASSEMBLY_COMPLETED", "ASSEMBLY_CANCELED", 1)
	req := newNotificationRequest(tampered, signature)

	if _, err := ParseAssemblyNotification(req, authSecret); err == nil {
		t.Fatal("expected signature verification to fail for a tampered payload")
	}
}

func TestParseAssemblyNotification_RejectsWrongSecret(t *testing.T) {
	signature := signPayload("foo_secret", examplePayload)
	req := newNotificationRequest(examplePayload, signature)

	if _, err := ParseAssemblyNotification(req, "wrong_secret"); err == nil {
		t.Fatal("expected signature verification to fail for the wrong auth secret")
	}
}
