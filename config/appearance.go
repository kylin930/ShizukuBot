package config

import (
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
)

const DefaultBackground = "https://teachermate.oss-cn-qingdao.aliyuncs.com/vokMW-1750340487715-nekoaft_N02_a.png"

type Background struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
type Theme struct {
	Mode         string `json:"mode"`
	PrimaryColor string `json:"primary_color"`
}
type Appearance struct {
	Background Background `json:"background"`
	Theme      Theme      `json:"theme"`
}

func DefaultAppearance() Appearance {
	return Appearance{Background: Background{Type: "default"}, Theme: Theme{Mode: "auto"}}
}

var ColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var UploadPattern = regexp.MustCompile(`^/appearance/images/[0-9a-f]{32}\.(png|jpg|webp)$`)

func (a Appearance) Validate() error {
	if a.Theme.Mode != "auto" && a.Theme.Mode != "light" && a.Theme.Mode != "dark" {
		return errors.New("invalid theme mode")
	}
	if a.Theme.PrimaryColor != "" && !ColorPattern.MatchString(a.Theme.PrimaryColor) {
		return errors.New("primary color must be #RRGGBB")
	}
	b := a.Background
	switch b.Type {
	case "default":
		if b.Value != "" {
			return errors.New("default background must have an empty value")
		}
	case "upload":
		if !UploadPattern.MatchString(b.Value) {
			return errors.New("invalid uploaded background")
		}
	case "url":
		if len(b.Value) > 2048 || strings.TrimSpace(b.Value) != b.Value {
			return errors.New("invalid background URL")
		}
		u, err := url.Parse(b.Value)
		if err != nil || u.User != nil {
			return errors.New("invalid background URL")
		}
		if u.Scheme == "https" && u.Hostname() != "" {
			return nil
		}
		if u.Scheme == "" && u.Host == "" && strings.HasPrefix(b.Value, "/static/") && path.Clean(u.Path) == u.Path && !strings.ContainsAny(b.Value, "%\\") && u.RawQuery == "" && u.Fragment == "" {
			switch strings.ToLower(path.Ext(u.Path)) {
			case ".png", ".jpg", ".jpeg", ".webp":
				return nil
			}
		}
		return errors.New("background requires HTTPS or a safe /static/ image path")
	default:
		return errors.New("invalid background type")
	}
	return nil
}
func (a Appearance) BackgroundURL() string {
	if a.Background.Type == "default" {
		return DefaultBackground
	}
	return a.Background.Value
}
