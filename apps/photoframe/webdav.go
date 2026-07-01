package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// maxErrorBodyBytes caps how much of an unexpected response body we read back
// into the error message.
const maxErrorBodyBytes = 2048

var (
	errNotAbsoluteURL   = errors.New("WEBDAV_BASE_URL must be an absolute URL")
	errUnexpectedStatus = errors.New("unexpected PROPFIND status")
)

// propfindBody requests just the properties we need to decide whether an entry
// is a displayable image.
const propfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:resourcetype/>
    <d:getcontenttype/>
    <d:getlastmodified/>
  </d:prop>
</d:propfind>`

// multistatus mirrors the relevant parts of a WebDAV PROPFIND response.
type multistatus struct {
	XMLName   xml.Name      `xml:"multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string        `xml:"href"`
	Propstat []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Status string  `xml:"status"`
	Prop   davProp `xml:"prop"`
}

type davProp struct {
	ContentType  string          `xml:"getcontenttype"`
	LastModified string          `xml:"getlastmodified"`
	ResourceType davResourceType `xml:"resourcetype"`
}

type davResourceType struct {
	Collection *struct{} `xml:"collection"`
}

// WebDAVClient lists and fetches files from a WebDAV endpoint, optionally
// authenticating through Cloudflare Access.
type WebDAVClient struct {
	cfg    Config
	http   *http.Client
	origin string // scheme://host derived from the base URL
}

// NewWebDAVClient builds a client from config. The HTTP client is shared and
// reused across requests.
func NewWebDAVClient(cfg Config, hc *http.Client) (*WebDAVClient, error) {
	u, err := url.Parse(cfg.WebDAVBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid WEBDAV_BASE_URL: %w", err)
	}

	if u.Scheme == "" || u.Host == "" {
		return nil, errNotAbsoluteURL
	}

	return &WebDAVClient{
		cfg:    cfg,
		http:   hc,
		origin: u.Scheme + "://" + u.Host,
	}, nil
}

// List performs a Depth:1 PROPFIND on the configured folder and returns the
// server-absolute hrefs of the image files it contains.
func (c *WebDAVClient) List(ctx context.Context) ([]string, error) {
	// PROPFIND must target the collection URL with a trailing slash. Without it
	// some servers (Nextcloud/SabreDAV) reply 301/302 to the canonical "/" form,
	// and Go's client follows redirects by downgrading PROPFIND to GET — the
	// response is then no longer 207 and the listing fails.
	target := c.cfg.WebDAVBaseURL + c.cfg.WebDAVPath
	if !strings.HasSuffix(target, "/") {
		target += "/"
	}

	req, err := http.NewRequestWithContext(ctx, "PROPFIND", target, strings.NewReader(propfindBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	c.applyAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMultiStatus {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		snippet := strings.TrimSpace(string(body))

		return nil, fmt.Errorf("PROPFIND %s: %w %s: %s", target, errUnexpectedStatus, resp.Status, snippet)
	}

	var ms multistatus

	err = xml.NewDecoder(resp.Body).Decode(&ms)
	if err != nil {
		return nil, fmt.Errorf("decode PROPFIND response: %w", err)
	}

	// The folder itself is returned as one of the responses; skip it and any
	// sub-collections, keeping only image files.
	requestPath := mustPath(target)

	var images []string

	for _, r := range ms.Responses {
		if href, ok := imageHref(r, requestPath); ok {
			images = append(images, href)
		}
	}

	return images, nil
}

// Fetch streams a single file (identified by its server-absolute href) from the
// WebDAV endpoint. The caller owns closing the returned body.
func (c *WebDAVClient) Fetch(ctx context.Context, href string) (*http.Response, error) {
	target := c.origin + href

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}

	c.applyAuth(req)

	return c.http.Do(req)
}

// applyAuth adds Basic auth plus optional Cloudflare Access service-token
// headers to an outbound request.
func (c *WebDAVClient) applyAuth(req *http.Request) {
	req.SetBasicAuth(c.cfg.WebDAVUsername, c.cfg.WebDAVPassword)

	if c.cfg.CFAccessClientID != "" {
		// Go canonicalises header keys on the wire, so the "Cf-" casing here is
		// equivalent to Cloudflare's documented "CF-Access-Client-*".
		req.Header.Set("Cf-Access-Client-Id", c.cfg.CFAccessClientID)
		req.Header.Set("Cf-Access-Client-Secret", c.cfg.CFAccessClientSecret)
	}
}

// imageHref returns the href of a PROPFIND response entry if it is a displayable
// image file (not the folder itself or a sub-collection).
func imageHref(r davResponse, requestPath string) (string, bool) {
	href := strings.TrimSpace(r.Href)
	if href == "" {
		return "", false
	}

	prop, ok := okProp(r)
	if !ok || prop.ResourceType.Collection != nil {
		return "", false
	}

	if !isImage(prop.ContentType, href) {
		return "", false
	}

	if samePath(href, requestPath) {
		return "", false
	}

	return href, true
}

func okProp(r davResponse) (davProp, bool) {
	for _, ps := range r.Propstat {
		if strings.Contains(ps.Status, "200") {
			return ps.Prop, true
		}
	}

	var zero davProp

	return zero, false
}

// isImage decides whether an entry is a displayable image, preferring the
// reported content type and falling back to the file extension.
func isImage(contentType, href string) bool {
	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return true
	}

	if contentType != "" {
		return false
	}

	switch strings.ToLower(path.Ext(decodePath(href))) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".avif", ".heic":
		return true
	}

	return false
}

// mustPath extracts the path component of a URL, falling back to the raw value.
func mustPath(raw string) string {
	u, err := url.Parse(raw)
	if err == nil {
		return strings.TrimRight(u.EscapedPath(), "/")
	}

	return strings.TrimRight(raw, "/")
}

// samePath compares an href against a path, tolerating trailing slashes.
func samePath(href, p string) bool {
	return strings.TrimRight(href, "/") == strings.TrimRight(p, "/")
}

// decodePath best-effort percent-decodes a path for extension inspection.
func decodePath(href string) string {
	d, err := url.PathUnescape(href)
	if err == nil {
		return d
	}

	return href
}
