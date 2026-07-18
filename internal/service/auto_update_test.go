package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsNewerRelease(t *testing.T) {
	for _, test := range []struct {
		latest  string
		current string
		want    bool
	}{
		{"v0.12.2", "v0.12.1", true},
		{"v0.12.2", "v0.12.2", false},
		{"v0.12.1", "v0.12.2", false},
		{"v1.0.0", "v1.0.0-rc.1", true},
		{"v1.0.0-rc.1", "v1.0.0", false},
		{"invalid", "v1.0.0", false},
	} {
		if got := isNewerRelease(test.latest, test.current); got != test.want {
			t.Errorf("isNewerRelease(%q, %q) = %v, want %v", test.latest, test.current, got, test.want)
		}
	}
}

func TestFetchCandidateMatchesCurrentPlatform(t *testing.T) {
	previousURL, previousVersion := autoUpdateRepositoryURL, BuildVersion
	t.Cleanup(func() {
		autoUpdateRepositoryURL = previousURL
		BuildVersion = previousVersion
	})
	BuildVersion = "v0.12.1"
	assetName := fmt.Sprintf("flai-community-v0.12.2-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = fmt.Fprintf(writer, `{"tag_name":"v0.12.2","assets":[{"name":%q,"browser_download_url":"https://example.test/download","digest":"sha256:%x"}]}`,
			assetName, sha256.Sum256([]byte("release")))
	}))
	defer server.Close()
	autoUpdateRepositoryURL = server.URL

	updater := NewAutoUpdateService()
	updater.client = server.Client()
	candidate, err := updater.fetchCandidate(context.Background())
	if err != nil {
		t.Fatalf("fetchCandidate() error = %v", err)
	}
	if candidate.Version != "v0.12.2" || candidate.Asset.Name != assetName {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
}

func TestExtractZipBinaryRequiresExactlyOneRootFile(t *testing.T) {
	directory := t.TempDir()
	archivePath := filepath.Join(directory, "release.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	entry, err := writer.Create("flai-community")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("binary")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(directory, "flai-community.update")
	if err := extractZipBinary(archivePath, target); err != nil {
		t.Fatalf("extractZipBinary() error = %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "binary" {
		t.Fatalf("extracted content = %q, error = %v", content, err)
	}
}
