package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsImage(t *testing.T) {
	cases := []struct {
		contentType string
		href        string
		want        bool
	}{
		{"image/jpeg", "/a/b.jpg", true},
		{"IMAGE/PNG", "/a/b.png", true},
		{"text/plain", "/a/b.txt", false},
		{"application/octet-stream", "/a/b.jpg", false}, // explicit non-image type wins
		{"", "/a/photo.JPG", true},                      // fall back to extension
		{"", "/a/notes.pdf", false},
		{"", "/a/encoded%20name.webp", true},
	}
	for _, c := range cases {
		if got := isImage(c.contentType, c.href); got != c.want {
			t.Errorf("isImage(%q, %q) = %v, want %v", c.contentType, c.href, got, c.want)
		}
	}
}

const samplePropfind = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/remote.php/dav/files/user/Frame/</d:href>
    <d:propstat><d:status>HTTP/1.1 200 OK</d:status>
      <d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/remote.php/dav/files/user/Frame/a.jpg</d:href>
    <d:propstat><d:status>HTTP/1.1 200 OK</d:status>
      <d:prop><d:resourcetype/><d:getcontenttype>image/jpeg</d:getcontenttype></d:prop>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/remote.php/dav/files/user/Frame/notes.txt</d:href>
    <d:propstat><d:status>HTTP/1.1 200 OK</d:status>
      <d:prop><d:resourcetype/><d:getcontenttype>text/plain</d:getcontenttype></d:prop>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/remote.php/dav/files/user/Frame/sub/</d:href>
    <d:propstat><d:status>HTTP/1.1 200 OK</d:status>
      <d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>
    </d:propstat>
  </d:response>
</d:multistatus>`

func TestList(t *testing.T) {
	var gotAuth, gotCFID, gotDepth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			t.Errorf("method = %s, want PROPFIND", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotCFID = r.Header.Get("CF-Access-Client-Id")
		gotDepth = r.Header.Get("Depth")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(samplePropfind))
	}))
	defer srv.Close()

	cfg := Config{
		WebDAVBaseURL:        srv.URL + "/remote.php/dav/files/user",
		WebDAVPath:           "/Frame",
		WebDAVUsername:       "user",
		WebDAVPassword:       "pass",
		CFAccessClientID:     "cf-id",
		CFAccessClientSecret: "cf-secret",
		RequestTimeout:       5 * time.Second,
	}
	c, err := NewWebDAVClient(cfg, srv.Client())
	if err != nil {
		t.Fatalf("NewWebDAVClient: %v", err)
	}

	images, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(images) != 1 || images[0] != "/remote.php/dav/files/user/Frame/a.jpg" {
		t.Fatalf("images = %v, want exactly the one jpg", images)
	}
	if gotAuth == "" {
		t.Error("expected Authorization header")
	}
	if gotCFID != "cf-id" {
		t.Errorf("CF-Access-Client-Id = %q, want cf-id", gotCFID)
	}
	if gotDepth != "1" {
		t.Errorf("Depth = %q, want 1", gotDepth)
	}
}
