package main

import (
	"bytes"
	"crypto/rand"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

/* Examples

// start session
curl -v -H 'app-id:c2842d1b-ad5c-47c6-b28f-cc495abd7d32' -X POST 'http://127.0.0.1:8989/start_session?session=123'

// upload/update
curl -v -H 'session:123' -X PUT --data-binary @test.bmp 'http://127.0.0.1:8989/notifications/abc/0'

// fetch
curl -v -H 'session:123' 'http://127.0.0.1:8989/notifications/abc/0' -o test2.bmp

// end session
curl -v -H 'app-id:c2842d1b-ad5c-47c6-b28f-cc495abd7d32' -X POST 'http://127.0.0.1:8989/end_session?session=123'
*/

var sessionId = uuid.New().String()

func generateRandomBytes(len int) []byte {
	buf := make([]byte, len)
	// then we can call rand.Read.
	_, err := rand.Read(buf)
	if err != nil {
		log.Fatalf("error while generating random string: %s", err)
	}
	return buf
}

func compareByteArray(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func startSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/start_session?session="+sessionId, nil)
	req.Header.Set("app-id", appId)
	w := httptest.NewRecorder()
	sessionStart(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Failed to start session. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()
}

func endSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/end_session?session="+sessionId, nil)
	req.Header.Set("app-id", appId)
	w := httptest.NewRecorder()
	sessionEnd(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Failed to end session. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()
}

func TestSession(t *testing.T) {
	startSession(t)

	// incorrect: same session ID
	req := httptest.NewRequest(http.MethodPost, "/start_session?session="+sessionId, nil)
	req.Header.Set("app-id", appId)
	w := httptest.NewRecorder()
	sessionStart(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Repeated session request should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// close an invalid session
	req = httptest.NewRequest(http.MethodPost, "/end_session?session=should_not_be_there", nil)
	req.Header.Set("app-id", appId)
	w = httptest.NewRecorder()
	sessionEnd(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("End non-existent session should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// close the correct one
	endSession(t)

	// wrong URL in start_session
	req = httptest.NewRequest(http.MethodPost, "/start_session", nil)
	req.Header.Set("app-id", appId)
	w = httptest.NewRecorder()
	sessionStart(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Start invalid session should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// wrong URL in end_session
	req = httptest.NewRequest(http.MethodPost, "/end_session", nil)
	req.Header.Set("app-id", appId)
	w = httptest.NewRecorder()
	sessionEnd(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("End invalid session should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// wrong app id in start_session
	req = httptest.NewRequest(http.MethodPost, "/start_session?session="+sessionId, nil)
	req.Header.Set("app-id", "something else")
	w = httptest.NewRecorder()
	sessionStart(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("Invalid app-id in header should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// wrong app id in end_session
	req = httptest.NewRequest(http.MethodPost, "/end_session?session="+sessionId, nil)
	req.Header.Set("app-id", "something else")
	w = httptest.NewRecorder()
	sessionEnd(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("Invalid app-id in header should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()
}

func TestNotifications(t *testing.T) {
	startSession(t)

	id := uuid.New().String()

	// wrong method
	req := httptest.NewRequest(http.MethodPost, "/notifications/"+id+"/0", nil)
	req.Header.Set("session", sessionId)
	w := httptest.NewRecorder()
	serveNotification(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST should not be allowed. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// wrong URL
	req = httptest.NewRequest(http.MethodPost, "/notifications/"+id, nil)
	req.Header.Set("session", sessionId)
	w = httptest.NewRecorder()
	serveNotification(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("URL should be invalid. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// upload some random binaries
	bitmap1 := generateRandomBytes(300)
	req = httptest.NewRequest(http.MethodPut, "/notifications/"+id+"/0", bytes.NewBuffer(bitmap1))
	req.Header.Set("session", sessionId)
	w = httptest.NewRecorder()
	serveNotification(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Failed to upload bitmap 1. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// upload some random binaries to page 1
	bitmap2 := generateRandomBytes(400)
	req = httptest.NewRequest(http.MethodPut, "/notifications/"+id+"/1", bytes.NewBuffer(bitmap2))
	req.Header.Set("session", sessionId)
	w = httptest.NewRecorder()
	serveNotification(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Failed to upload bitmap 2. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// download bitmap 1 and compare
	req = httptest.NewRequest(http.MethodGet, "/notifications/"+id+"/0", nil)
	req.Header.Set("session", sessionId)
	w = httptest.NewRecorder()
	serveNotification(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Failed to get bitmap 2. HTTP status:%d", res.StatusCode)
	}
	downloadedBitmap1, err := io.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	if !compareByteArray(downloadedBitmap1, bitmap1) {
		t.Error("downloaded bitmap 1 is wrong")
	}
	res.Body.Close()

	// download bitmap 2 and compare
	req = httptest.NewRequest(http.MethodGet, "/notifications/"+id+"/1", nil)
	req.Header.Set("session", sessionId)
	w = httptest.NewRecorder()
	serveNotification(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Failed to get bitmap 2. HTTP status:%d", res.StatusCode)
	}
	downloadedBitmap2, err := io.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	if !compareByteArray(downloadedBitmap2, bitmap2) {
		t.Error("downloaded bitmap 2 is wrong")
	}
	res.Body.Close()

	// download non-existent bitmap
	req = httptest.NewRequest(http.MethodGet, "/notifications/"+id+"/2", nil)
	req.Header.Set("session", sessionId)
	w = httptest.NewRecorder()
	serveNotification(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("downloading non-existent page should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// upload to wrong page
	bitmap3 := generateRandomBytes(100)
	req = httptest.NewRequest(http.MethodPut, "/notifications/"+id+"/5", bytes.NewBuffer(bitmap3))
	req.Header.Set("session", sessionId)
	w = httptest.NewRecorder()
	serveNotification(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("uploading to wrong page should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	// non-existent notification id
	req = httptest.NewRequest(http.MethodGet, "/notifications/should_not_exist/0", nil)
	req.Header.Set("session", sessionId)
	w = httptest.NewRecorder()
	serveNotification(w, req)
	res = w.Result()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("downloading non-existent notification should fail. HTTP status:%d", res.StatusCode)
	}
	res.Body.Close()

	endSession(t)
}