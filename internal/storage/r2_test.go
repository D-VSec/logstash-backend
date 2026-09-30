package storage

import "testing"

func TestNewR2UploaderRequiresCredentials(t *testing.T) {
	if _, err := NewR2Uploader("https://example.com", "logs", "archive", "", "secret"); err == nil {
		t.Fatal("expected missing access key error")
	}
}

func TestNewR2Uploader(t *testing.T) {
	uploader, err := NewR2Uploader("https://example.com", "logs", "archive", "key", "secret")
	if err != nil {
		t.Fatalf("new R2 uploader: %v", err)
	}
	if uploader.bucket != "logs" || uploader.prefix != "archive" {
		t.Fatalf("unexpected uploader: %+v", uploader)
	}
}
