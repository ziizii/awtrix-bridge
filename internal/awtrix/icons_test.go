package awtrix

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToGIFKeepsColors(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	src.Set(1, 2, color.NRGBA{255, 170, 0, 255})
	src.Set(5, 5, color.NRGBA{0, 200, 255, 255})
	src.Set(6, 6, color.NRGBA{255, 255, 255, 0}) // transparent: off
	data, err := ToGIF(src)
	if err != nil {
		t.Fatal(err)
	}
	img, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	want := map[[2]int][3]uint8{{1, 2}: {255, 170, 0}, {5, 5}: {0, 200, 255}, {6, 6}: {0, 0, 0}, {0, 0}: {0, 0, 0}}
	for p, w := range want {
		r, g, b, _ := img.At(p[0], p[1]).RGBA()
		if got := [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}; got != w {
			t.Errorf("pixel %v: %v, want %v", p, got, w)
		}
	}
	// A larger image is scaled to 8x8.
	if data, _ := ToGIF(image.NewNRGBA(image.Rect(0, 0, 32, 32))); data == nil {
		t.Error("32x32 not converted")
	} else if cfg, _ := gif.DecodeConfig(bytes.NewReader(data)); cfg.Width != 8 || cfg.Height != 8 {
		t.Errorf("size %dx%d", cfg.Width, cfg.Height)
	}
}

func TestClockHTTPIcons(t *testing.T) {
	var uploaded, name string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dir") != "/ICONS" {
			http.Error(w, "dir", 400)
			return
		}
		if r.Method == "GET" {
			io.WriteString(w, `{"files":[{"name":"11999.gif","size":83},{"name":"6401.jpg","size":800}]}`)
			return
		}
		_, params, _ := strings.Cut(r.Header.Get("Content-Type"), "boundary=")
		mr := multipart.NewReader(r.Body, params)
		p, _ := mr.NextPart()
		b, _ := io.ReadAll(p)
		uploaded, name = string(b), p.FormName()+" "+p.FileName()
	}))
	defer srv.Close()
	c := ClockHTTP{Base: srv.URL, Client: srv.Client()}
	icons, err := c.Icons(context.Background())
	if err != nil || !icons["11999"] || !icons["6401"] || icons["2644"] {
		t.Fatalf("icons %v %v", icons, err)
	}
	if err := c.UploadIcon(context.Background(), "2644", []byte("GIF89a")); err != nil || uploaded != "GIF89a" || name != "file 2644.gif" {
		t.Errorf("upload: %v %q %q", err, uploaded, name)
	}
	if DeviceIP([]byte(`{"ipAddress":"192.168.11.199","uid":"x"}`)) != "192.168.11.199" {
		t.Error("device ip")
	}
}
