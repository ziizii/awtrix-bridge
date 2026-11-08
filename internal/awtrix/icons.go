package awtrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg" // LaMetric serves PNG, GIF and sometimes JPEG
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The clock draws only icons stored on it, as /ICONS/<id>.gif (or .jpg,
// .png); an unknown id shows no icon. Icon ids are LaMetric icon ids, so
// a missing icon can be fetched from LaMetric and uploaded, the way the
// clock's own web page does it.

// IconID is a LaMetric icon id.
var IconID = regexp.MustCompile(`^[0-9]{1,7}$`)

// ClockHTTP is the clock's HTTP API, for what MQTT cannot do (files).
type ClockHTTP struct {
	Base   string // "http://192.168.11.199"
	Client *http.Client
}

// DeviceIP reads the clock's address from its device state payload.
func DeviceIP(payload []byte) string {
	var d struct {
		IP string `json:"ipAddress"`
	}
	_ = json.Unmarshal(payload, &d)
	return d.IP
}

// Icons lists the icon ids stored on the clock.
func (c ClockHTTP) Icons(ctx context.Context) (map[string]bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.Base+"/api/v1/files?dir="+url.QueryEscape("/ICONS"), nil)
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var list struct {
		Files []struct{ Name string } `json:"files"`
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list icons: HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("list icons: %w", err)
	}
	out := map[string]bool{}
	for _, f := range list.Files {
		id, _, _ := strings.Cut(f.Name, ".")
		out[id] = true
	}
	return out, nil
}

// UploadIcon stores a GIF on the clock as /ICONS/<id>.gif.
func (c ClockHTTP) UploadIcon(ctx context.Context, id string, data []byte) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", id+".gif")
	part.Write(data)
	w.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", c.Base+"/api/v1/files?dir="+url.QueryEscape("/ICONS"), &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("upload icon %s: HTTP %d %s", id, resp.StatusCode, msg)
	}
	return nil
}

// LaMetricIconURL is where LaMetric serves an icon's 8×8 image.
func LaMetricIconURL(id string) string {
	return "https://developer.lametric.com/content/apps/icon_thumbs/" + id
}

// FetchIcon downloads a LaMetric icon as a GIF the clock can draw:
// animated GIFs as they are, other images converted to an 8×8 GIF.
func FetchIcon(ctx context.Context, client *http.Client, id string) ([]byte, error) {
	if !IconID.MatchString(id) {
		return nil, fmt.Errorf("icon id must be a LaMetric number, got %q", id)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", LaMetricIconURL(id), nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("LaMetric has no icon %s", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch icon %s: HTTP %d", id, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte("GIF8")) {
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("icon %s: %w", id, err)
	}
	return ToGIF(img)
}

// ToGIF converts an image to an 8×8 GIF with its exact colors (an 8×8
// image has at most 64); transparent pixels become black, which is off
// on the clock.
func ToGIF(src image.Image) ([]byte, error) {
	const size = 8
	b := src.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil, errors.New("empty image")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(rgba, rgba.Bounds(), image.Black, image.Point{}, draw.Src)
	for y := range size {
		for x := range size {
			c := src.At(b.Min.X+x*b.Dx()/size, b.Min.Y+y*b.Dy()/size) // nearest pixel
			draw.Draw(rgba, image.Rect(x, y, x+1, y+1), image.NewUniform(c), image.Point{}, draw.Over)
		}
	}
	pal := color.Palette{color.Black}
	seen := map[color.Color]bool{color.RGBA{0, 0, 0, 255}: true}
	for y := range size {
		for x := range size {
			if c := rgba.RGBAAt(x, y); !seen[c] {
				seen[c] = true
				pal = append(pal, c)
			}
		}
	}
	out := image.NewPaletted(rgba.Bounds(), pal)
	draw.Draw(out, out.Bounds(), rgba, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, out, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
