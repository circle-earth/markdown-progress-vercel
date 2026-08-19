package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"
)

type Data struct {
	BackgroundColor    string
	Label              string
	Progress           int
	PickedColor        string
	Detached           bool
	HasCustomTextColor bool
	TotalWidth         float64
	TextX              float64
	TextAnchor         string
	TextColor          string
}

const (
	minPercentage     = 0.0
	maxPercentage     = 100.0
	totalBarWidth     = 90.0
	cacheControlValue = "public, max-age=300, s-maxage=300"
	maxLabelRunes     = 64

	svgTemplate = `<svg width="{{.TotalWidth}}" height="20" xmlns="http://www.w3.org/2000/svg">
  <style>
    .progress-text {
      font-family: DejaVu Sans,Verdana,Geneva,sans-serif;
      font-size: 11px;
      fill: {{.TextColor}};
    }
    {{if and .Detached (not .HasCustomTextColor)}}
    @media (prefers-color-scheme: dark) {
      .progress-text {
        fill: #e6edf3;
      }
    }
    @media (prefers-color-scheme: light) {
      .progress-text {
        fill: #24292f;
      }
    }
    {{end}}
  </style>
  <linearGradient id="a" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".2"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <rect rx="4" x="0" width="90.0" height="20" fill="{{.BackgroundColor}}"/>
  <rect rx="4" x="0" width="{{.Progress}}" height="20" fill="{{.PickedColor}}"/>
  <rect rx="4" width="90.0" height="20" fill="url(#a)"/>
  <g text-anchor="{{.TextAnchor}}">
    <text class="progress-text" x="{{.TextX}}" y="14">
      {{.Label}}
    </text>
  </g>
</svg>`
)

var (
	grey   = "#555"
	red    = "#d9534f"
	yellow = "#f0ad4e"
	green  = "#5cb85c"

	hexColorPattern  = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
	progressTemplate = template.Must(template.New("progress").Parse(svgTemplate))

	svgWidthRegex  = regexp.MustCompile(`(?i)\bwidth="[^"]*"`)
	svgHeightRegex = regexp.MustCompile(`(?i)\bheight="[^"]*"`)

	// Default fallback SVG for any unknown IDE request
	defaultIDEIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#007ACC"><rect width="24" height="24" rx="5" fill="#1E1E1E"/><path fill="#007ACC" d="M4 4h16v16H4V4zm2 2v12h12V6H6zm2 2h3v2H8V8zm0 3h5v2H8v-2zm0 3h7v2H8v-2z"/></svg>`

	// 100% In-Memory Official Standalone Vector SVGs for ALL IDEs & Editors (0ms Network Latency)
	customIDEIcons = map[string]string{
		"vscode":             `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#0065A9" d="M18.5 2.5L14.7.1c-.6-.4-1.4-.2-1.8.4L1.7 15.3c-.4.6-.2 1.4.4 1.8l1.1.7c.6.4 1.4.2 1.8-.4L18 3.5c.5-.7 1.5-.7 1.9 0l.6 1c.4.6.2 1.4-.4 1.8L4.3 20.7c-.6.4-.8 1.2-.4 1.8l1.1 1.7c.4.6 1.2.8 1.8.4L21.5 9.7c.6-.4.8-1.2.4-1.8L18.5 2.5z"/><path fill="#007ACC" d="M18.5 21.5l-3.8 2.4c-.6.4-1.4.2-1.8-.4L1.7 8.7c-.4-.6-.2-1.4.4-1.8l1.1-.7c.6-.4 1.4-.2 1.8.4L18 20.5c.5.7 1.5.7 1.9 0l.6-1c.4-.6.2-1.4-.4-1.8L4.3 3.3c-.6-.4-.8-1.2-.4-1.8l1.1-1.7c.4-.6 1.2-.8 1.8-.4L21.5 14.3c.6.4.8 1.2.4 1.8l-3.4 5.4z"/><path fill="#1F9CF0" d="M14.7 23.9c-.6.4-1.4.2-1.8-.4V.5c0-.7.8-1.1 1.4-.7l7.2 3.6c.4.2.7.6.7 1.1v15c0 .5-.3.9-.7 1.1l-6.8 3.3z"/></svg>`,
		"visual-studio-code": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#0065A9" d="M18.5 2.5L14.7.1c-.6-.4-1.4-.2-1.8.4L1.7 15.3c-.4.6-.2 1.4.4 1.8l1.1.7c.6.4 1.4.2 1.8-.4L18 3.5c.5-.7 1.5-.7 1.9 0l.6 1c.4.6.2 1.4-.4 1.8L4.3 20.7c-.6.4-.8 1.2-.4 1.8l1.1 1.7c.4.6 1.2.8 1.8.4L21.5 9.7c.6-.4.8-1.2.4-1.8L18.5 2.5z"/><path fill="#007ACC" d="M18.5 21.5l-3.8 2.4c-.6.4-1.4.2-1.8-.4L1.7 8.7c-.4-.6-.2-1.4.4-1.8l1.1-.7c.6-.4 1.4-.2 1.8.4L18 20.5c.5.7 1.5.7 1.9 0l.6-1c.4-.6.2-1.4-.4-1.8L4.3 3.3c-.6-.4-.8-1.2-.4-1.8l1.1-1.7c.4-.6 1.2-.8 1.8-.4L21.5 14.3c.6.4.8 1.2.4 1.8l-3.4 5.4z"/><path fill="#1F9CF0" d="M14.7 23.9c-.6.4-1.4.2-1.8-.4V.5c0-.7.8-1.1 1.4-.7l7.2 3.6c.4.2.7.6.7 1.1v15c0 .5-.3.9-.7 1.1l-6.8 3.3z"/></svg>`,
		"cursor":             `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#000000"><path d="M11.503.131 1.891 5.678a.84.84 0 0 0-.42.726v11.188c0 .3.162.575.42.724l9.609 5.55a1 1 0 0 0 .998 0l9.61-5.55a.84.84 0 0 0 .42-.724V6.404a.84.84 0 0 0-.42-.726L12.497.131a1 1 0 0 0-.994 0z"/></svg>`,
		"claude":             `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#D97757"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>`,
		"anthropic":          `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#D97757"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>`,
		"claudecode":         `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#D97757"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>`,
		"antigravity":        `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><defs><linearGradient id="ag" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#00F2FE"/><stop offset="100%" stop-color="#4FACFE"/></linearGradient></defs><rect width="24" height="24" rx="6" fill="url(#ag)"/><path fill="#FFFFFF" d="M12 4L6 18h3.5l1.2-3.3h6.6l1.2 3.3H22L16 4h-4zm.1 4.5l2.2 6.2H9.7l2.4-6.2z"/></svg>`,
		"acode":              `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#1E88E5"/><path fill="#FFA000" d="M7 6l-5 6 5 6 1.4-1.4L4.8 12l3.6-4.6L7 6zm10 0l-1.4 1.4 3.6 4.6-3.6 4.6L17 18l5-6-5-6zM13.4 4l-4.8 16h2.1l4.8-16h-2.1z"/></svg>`,
		"intellij":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="ij" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#FE2857"/><stop offset="50%" stop-color="#9B00E8"/><stop offset="100%" stop-color="#000000"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#ij)" rx="3"/><path fill="#FFFFFF" d="M5 15h3v2H5v-2zm0-8h3v5H5V7zm5 0h6v2h-4v2h4v2h-4v3h-2V7z"/></svg>`,
		"idea":               `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="ij2" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#FE2857"/><stop offset="50%" stop-color="#9B00E8"/><stop offset="100%" stop-color="#000000"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#ij2)" rx="3"/><path fill="#FFFFFF" d="M5 15h3v2H5v-2zm0-8h3v5H5V7zm5 0h6v2h-4v2h4v2h-4v3h-2V7z"/></svg>`,
		"pycharm":            `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="pc" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#21D789"/><stop offset="100%" stop-color="#000000"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#pc)" rx="3"/><path fill="#FFFFFF" d="M5 7h4c1.1 0 2 .9 2 2s-.9 2-2 2H7v3H5V7zm2 2v2h2c.6 0 1-.4 1-1s-.4-1-1-1H7zm5-2h2l2 4 2-4h2v7h-2v-4l-2 4h-1l-2-4v4h-1V7z"/></svg>`,
		"webstorm":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="ws" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#00CDD7"/><stop offset="100%" stop-color="#0080FF"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#ws)" rx="3"/><path fill="#FFFFFF" d="M5 7h2l1 4 1-4h2l1 4 1-4h2l-2 7h-2l-1-4-1 4H7L5 7zm10 0h3c1.1 0 2 .9 2 2v1c0 1.1-.9 2-2 2h-1v2h-2V7zm2 2v2h1c.6 0 1-.4 1-1v-.5c0-.5-.4-.5-1-.5h-1z"/></svg>`,
		"phpstorm":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="ps" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#B052C0"/><stop offset="100%" stop-color="#6F2DA8"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#ps)" rx="3"/><path fill="#FFFFFF" d="M5 7h4c1.1 0 2 .9 2 2s-.9 2-2 2H7v3H5V7zm2 2v2h2c.6 0 1-.4 1-1s-.4-1-1-1H7zm5-2h4c1.1 0 2 .9 2 2v1c0 1.1-.9 2-2 2h-2v2h-2V7zm2 2v2h2c.6 0 1-.4 1-1v-.5c0-.5-.4-.5-1-.5h-2z"/></svg>`,
		"clion":              `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="cl" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#21D789"/><stop offset="100%" stop-color="#0091FF"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#cl)" rx="3"/><path fill="#FFFFFF" d="M5 7h5v2H7v3h3v2H5V7zm7 0h2v5h3v2h-5V7z"/></svg>`,
		"rider":              `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="rd" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#E21245"/><stop offset="100%" stop-color="#7B00E0"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#rd)" rx="3"/><path fill="#FFFFFF" d="M5 7h4c1.1 0 2 .9 2 2s-.9 2-2 2H7v3H5V7zm2 2v2h2c.6 0 1-.4 1-1s-.4-1-1-1H7zm5-2h4c1.1 0 2 .9 2 2s-.9 2-2 2l2 3h-2l-2-3h-2v3h-2V7zm2 2v2h2c.6 0 1-.4 1-1s-.4-1-1-1h-2z"/></svg>`,
		"rubymine":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="rm" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#FE2857"/><stop offset="100%" stop-color="#9B00E8"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#rm)" rx="3"/><path fill="#FFFFFF" d="M5 7h4c1.1 0 2 .9 2 2s-.9 2-2 2H7v3H5V7zm2 2v2h2c.6 0 1-.4 1-1s-.4-1-1-1H7zm5-2h2l2 4 2-4h2v7h-2v-4l-2 4h-1l-2-4v4h-1V7z"/></svg>`,
		"goland":             `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="gl" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#00ADD8"/><stop offset="100%" stop-color="#000000"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#gl)" rx="3"/><path fill="#FFFFFF" d="M5 7h5v2H7v1h3v2H7v1h3v2H5V7zm7 0h2v7h-2V7z"/></svg>`,
		"datagrip":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#000000"/><defs><linearGradient id="dg" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#21D789"/><stop offset="100%" stop-color="#000000"/></linearGradient></defs><rect x="3" y="3" width="18" height="18" fill="url(#dg)" rx="3"/><path fill="#FFFFFF" d="M5 7h4c1.7 0 3 1.3 3 3s-1.3 3-3 3H5V7zm2 2v2h2c.6 0 1-.4 1-1s-.4-1-1-1H7zm5-2h4c1.7 0 3 1.3 3 3s-1.3 3-3 3h-4V7zm2 2v2h2c.6 0 1-.4 1-1s-.4-1-1-1h-2z"/></svg>`,
		"fleet":              `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#6366F1"><path d="M4 6h16v3H4V6zm0 6h12v3H4v-3zm0 6h8v3H4v-3z"/></svg>`,
		"androidstudio":      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#3DDC84" d="M12 2a10 10 0 100 20 10 10 0 000-20zm-1.5 5h3v2h-3V7zm-2 3h7v2h-7v-2zm-1 3h9v2H7.5v-2zm2 3h5v2h-5v-2z"/></svg>`,
		"android-studio":     `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#3DDC84" d="M12 2a10 10 0 100 20 10 10 0 000-20zm-1.5 5h3v2h-3V7zm-2 3h7v2h-7v-2zm-1 3h9v2H7.5v-2zm2 3h5v2h-5v-2z"/></svg>`,
		"xcode":              `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#147EFB"><path d="M12 2L2 19h20L12 2zm0 4l6.5 11h-13L12 6z"/></svg>`,
		"vim":                `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#019833"><path d="M2 2l10 20L22 2H2zm4 4h4l4 8 4-8h4L12 18 6 6z"/></svg>`,
		"neovim":             `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#57A143"><path d="M3 2l6 4v12l-6-4V2zm12 0l6 4v12l-6-4V2zM9 6l6 12h-3L6 6h3z"/></svg>`,
		"emacs":              `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#7F5AB6"><path d="M12 2A10 10 0 1022 12 10 10 0 0012 2zm1 14h-2v-4h2v4zm0-6h-2V8h2v2z"/></svg>`,
		"sublime":            `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#FF9800"><path d="M3 15.5l9 3.5 9-3.5-9-3.5-9 3.5zm0-7l9 3.5 9-3.5-9-3.5-9 3.5z"/></svg>`,
		"sublimetext":        `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#FF9800"><path d="M3 15.5l9 3.5 9-3.5-9-3.5-9 3.5zm0-7l9 3.5 9-3.5-9-3.5-9 3.5z"/></svg>`,
		"atom":               `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#66595C"><path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10 10-4.5 10-10S17.5 2 12 2zm0 18c-4.4 0-8-3.6-8-8s3.6-8 8-8 8 3.6 8 8-3.6 8-8 8z"/></svg>`,
		"replit":             `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#F26207"><path d="M2 1.5A1.5 1.5 0 013.5 0h7A1.5 1.5 0 0112 1.5v7A1.5 1.5 0 0110.5 10h-7A1.5 1.5 0 012 8.5v-7zM2 13.5A1.5 1.5 0 013.5 12h7a1.5 1.5 0 011.5 1.5v7a1.5 1.5 0 01-1.5 1.5h-7A1.5 1.5 0 012 20.5v-7zM14 1.5A1.5 1.5 0 0115.5 0h7A1.5 1.5 0 0124 1.5v7A1.5 1.5 0 0122.5 10h-7A1.5 1.5 0 0114 8.5v-7z"/></svg>`,
		"zed":                `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#FF5722"><path d="M5 5h14v3l-8 8h8v3H5v-3l8-8H5V5z"/></svg>`,
		"windsurf":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#09B6A2"><path d="M12 4L4 12l8 8 8-8-8-8zm0 4l4 4-4 4-4-4 4-4z"/></svg>`,
		"codeium":            `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#09B6A2"><path d="M12 4L4 12l8 8 8-8-8-8zm0 4l4 4-4 4-4-4 4-4z"/></svg>`,
		"vscodium":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#2F80ED"><path d="M23.15 2.5a1.5 1.5 0 00-1.8-.1L1.4 14.8a1.5 1.5 0 000 2.4l19.95 12.4a1.5 1.5 0 002.3-1.2V3.6a1.5 1.5 0 00-.5-1.1zM18 19.5L7.5 13 18 6.5v13z"/></svg>`,
		"codium":             `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#2F80ED"><path d="M23.15 2.5a1.5 1.5 0 00-1.8-.1L1.4 14.8a1.5 1.5 0 000 2.4l19.95 12.4a1.5 1.5 0 002.3-1.2V3.6a1.5 1.5 0 00-.5-1.1zM18 19.5L7.5 13 18 6.5v13z"/></svg>`,
		"warp":               `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#0066FF"><path d="M6 7l6 5-6 5V7zm6 10h6v2h-6v-2z"/></svg>`,
		"vs":                 `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#5C2D91"><path d="M17 2l-7 7-5-4L1 8l4 4-4 4 4 3 5-4 7 7 4-2V4l-4-2z"/></svg>`,
		"visualstudio":       `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#5C2D91"><path d="M17 2l-7 7-5-4L1 8l4 4-4 4 4 3 5-4 7 7 4-2V4l-4-2z"/></svg>`,
		"eclipse":            `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#2C2255"><path d="M12 2a10 10 0 100 20 10 10 0 000-20zm-2 3a7 7 0 110 14 7 7 0 010-14z"/></svg>`,
		"qt":                 `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#41CD52"><path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c2.3 0 4.4-.8 6.1-2.1l2.4 2.1 1.5-1.5-2.2-2C21.2 16.6 22 14.4 22 12c0-5.5-4.5-10-10-10zm0 16c-3.3 0-6-2.7-6-6s2.7-6 6-6 6 2.7 6 6-2.7 6-6 6z"/></svg>`,
		"qtcreator":          `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#41CD52"><path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c2.3 0 4.4-.8 6.1-2.1l2.4 2.1 1.5-1.5-2.2-2C21.2 16.6 22 14.4 22 12c0-5.5-4.5-10-10-10zm0 16c-3.3 0-6-2.7-6-6s2.7-6 6-6 6 2.7 6 6-2.7 6-6 6z"/></svg>`,
		"arduino":            `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#00979D"><path d="M6 7a5 5 0 000 10 5 5 0 003.5-8.5A5 5 0 0018 7a5 5 0 00-5 5 5 5 0 005 5 5 5 0 000-10 5 5 0 00-3.5 1.5A5 5 0 006 7zm0 3a2 2 0 110 4 2 2 0 010-4zm12 0a2 2 0 110 4 2 2 0 010-4z"/></svg>`,
		"xamarin":            `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#3498DB"><path d="M12 2L2 22h4l6-12 6 12h4L12 2z"/></svg>`,
		"terminal":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#4D4D4D"><rect width="24" height="24" rx="4" fill="#1E1E1E"/><path fill="#4CAF50" d="M5 7l5 5-5 5v-3l2-2-2-2V7zm7 8h7v2h-7v-2z"/></svg>`,
		"word":               `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#185ABD"><rect width="24" height="24" rx="4" fill="#185ABD"/><path fill="#FFF" d="M6 6h3.5l2.5 8 2.5-8H18l-4 12h-4L6 6z"/></svg>`,
		"excel":              `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#107C41"><rect width="24" height="24" rx="4" fill="#107C41"/><path fill="#FFF" d="M7 6h3l2 4 2-4h3l-3.5 6L17 18h-3l-2-4-2 4H7l3.5-6L7 6z"/></svg>`,
		"powerpoint":         `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#C43E1C"><rect width="24" height="24" rx="4" fill="#C43E1C"/><path fill="#FFF" d="M7 6h6c2.2 0 4 1.8 4 4s-1.8 4-4 4H10v4H7V6zm3 3v3h3c.8 0 1.5-.7 1.5-1.5S13.8 9 13 9h-3z"/></svg>`,
		"edge":               `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#0078D7"><path d="M12 2C6.5 2 2 6.5 2 12c0 4.4 2.8 8.2 6.8 9.5 2.7.9 5.8.5 8.2-1.1-2.1.2-4.2-.5-5.7-2-1.9-1.9-2.2-4.9-.7-7.1 1.2-1.7 3.3-2.6 5.4-2.3 2.1.3 3.9 1.9 4.6 3.9.7 2 0 4.2-1.6 5.5.9-1.3 1.2-3 1-4.5-.4-3.1-2.8-5.6-5.9-5.9H12z"/></svg>`,
		"textmate":           `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#FFC107"><rect width="24" height="24" rx="4" fill="#212121"/><path fill="#FFC107" d="M6 6h12v3H6V6zm0 5h12v3H6v-3zm0 5h8v3H6v-3z"/></svg>`,
		"nova":               `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#8E44AD"><rect width="24" height="24" rx="5" fill="#8E44AD"/><path fill="#FFF" d="M12 4l2.5 5.5L20 10.5l-4 4L17 20l-5-3-5 3 1-5.5-4-4 5.5-1L12 4z"/></svg>`,
		"wpsoffice":          `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#FF334B"><rect width="24" height="24" rx="4" fill="#FF334B"/><path fill="#FFF" d="M5 7l4 10 3-7 3 7 4-10h-3l-2.5 6.5L12 10l-2.5 3.5L7 7H5z"/></svg>`,
	}

	iconAliases = map[string]string{
		"node":       "file_type_node",
		"nodejs":     "file_type_node",
		"js":         "file_type_node",
		"javascript": "file_type_node",
		"py":         "file_type_python",
		"python":     "file_type_python",
		"go":         "file_type_go",
		"golang":     "file_type_go",
		"ts":         "file_type_typescript",
		"typescript": "file_type_typescript",
		"react":      "file_type_reactjs",
		"reactjs":    "file_type_reactjs",
		"jsx":        "file_type_reactjs",
		"tsx":        "file_type_reactts",
		"html":       "file_type_html",
		"css":        "file_type_css",
		"docker":     "file_type_docker",
		"dockerfile": "file_type_docker",
		"git":        "file_type_git",
		"json":       "file_type_json",
		"cpp":        "file_type_cpp",
		"c++":        "file_type_cpp",
		"c":          "file_type_c",
		"cs":         "file_type_csharp",
		"csharp":     "file_type_csharp",
		"java":       "file_type_java",
		"rust":       "file_type_rust",
		"rs":         "file_type_rust",
		"php":        "file_type_php",
		"vue":        "file_type_vue",
		"svelte":     "file_type_svelte",
		"tailwind":   "file_type_tailwind",
		"sass":       "file_type_scss",
		"scss":       "file_type_scss",
		"yaml":       "file_type_yaml",
		"yml":        "file_type_yaml",
		"md":         "file_type_markdown",
		"markdown":   "file_type_markdown",
	}
)

func setSVGSize(svg string, size int) string {
	if svgWidthRegex.MatchString(svg) {
		svg = svgWidthRegex.ReplaceAllString(svg, fmt.Sprintf(`width="%d"`, size))
	} else {
		svg = strings.Replace(svg, "<svg", fmt.Sprintf(`<svg width="%d"`, size), 1)
	}

	if svgHeightRegex.MatchString(svg) {
		svg = svgHeightRegex.ReplaceAllString(svg, fmt.Sprintf(`height="%d"`, size))
	} else {
		svg = strings.Replace(svg, "<svg", fmt.Sprintf(`<svg height="%d"`, size), 1)
	}
	return svg
}

func getRequestedSize(r *http.Request) int {
	sizeStr := r.URL.Query().Get("size")
	if sizeStr != "" {
		if sizeVal, err := strconv.Atoi(sizeStr); err == nil && sizeVal > 0 && sizeVal <= 512 {
			return sizeVal
		}
	}
	return 24 // Default size if omitted so markdown images render properly
}

func handleIDEIcon(w http.ResponseWriter, r *http.Request, ideName string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ideName = strings.ToLower(strings.TrimSpace(ideName))
	ideName = strings.TrimSuffix(ideName, ".svg")
	cleanName := regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(ideName, "")

	if ideName == "" {
		http.Error(w, "IDE name required", http.StatusBadRequest)
		return
	}

	// 100% In-Memory Lookup: Zero network requests, 0ms latency, no third-party dependency
	svgContent, exists := customIDEIcons[ideName]
	if !exists {
		svgContent, exists = customIDEIcons[cleanName]
	}
	if !exists {
		// Clean default fallback for unmapped IDE names
		svgContent = defaultIDEIcon
	}

	sizeVal := getRequestedSize(r)
	svgContent = setSVGSize(svgContent, sizeVal)

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=86400")

	if r.Method == http.MethodHead {
		return
	}

	_, _ = w.Write([]byte(svgContent))
}

func handleFileIcon(w http.ResponseWriter, r *http.Request, iconName string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	iconName = strings.ToLower(strings.TrimSpace(iconName))
	iconName = strings.TrimSuffix(iconName, ".svg")

	if iconName == "" {
		http.Error(w, "icon name required", http.StatusBadRequest)
		return
	}

	targetIcon := iconName
	if mapped, exists := iconAliases[iconName]; exists {
		targetIcon = mapped
	} else if !strings.HasPrefix(iconName, "file_type_") {
		targetIcon = "file_type_" + iconName
	}

	cdnURL := fmt.Sprintf("https://cdn.jsdelivr.net/gh/vscode-icons/vscode-icons@master/icons/%s.svg", targetIcon)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(cdnURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		fallbackURL := fmt.Sprintf("https://cdn.jsdelivr.net/gh/vscode-icons/vscode-icons@master/icons/%s.svg", iconName)
		resp, err = client.Get(fallbackURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			http.Error(w, "icon not found", http.StatusNotFound)
			return
		}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read icon", http.StatusInternalServerError)
		return
	}

	svgContent := string(bodyBytes)

	sizeVal := getRequestedSize(r)
	svgContent = setSVGSize(svgContent, sizeVal)

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", cacheControlValue)

	if r.Method == http.MethodHead {
		return
	}

	_, _ = w.Write([]byte(svgContent))
}

func clampPercentage(percentage float64) float64 {
	if percentage < minPercentage {
		return minPercentage
	}
	if percentage > maxPercentage {
		return maxPercentage
	}
	return percentage
}

func percentageToWidth(percentage float64) int {
	return int((totalBarWidth * percentage) / maxPercentage)
}

func parseOptionalColor(raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	if !hexColorPattern.MatchString(raw) {
		return "", false
	}
	return "#" + strings.ToLower(raw), true
}

func pickColor(percentage float64, successColor, warningColor, dangerColor string) string {
	pickedColor := green
	if successColor != "" {
		pickedColor = successColor
	}

	if percentage >= 0 && percentage < 33 {
		if dangerColor != "" {
			pickedColor = dangerColor
		} else {
			pickedColor = red
		}
	} else if percentage >= 33 && percentage < 70 {
		if warningColor != "" {
			pickedColor = warningColor
		} else {
			pickedColor = yellow
		}
	}

	return pickedColor
}

func formatNumber(value float64) string {
	formatted := strconv.FormatFloat(value, 'f', 2, 64)
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	if formatted == "-0" || formatted == "" {
		return "0"
	}
	return formatted
}

func formatPercentLabel(value float64) string {
	return formatNumber(value) + "%"
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ideParam := r.URL.Query().Get("ide")
	if ideParam != "" {
		handleIDEIcon(w, r, ideParam)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/ide/") {
		ideName := strings.TrimPrefix(r.URL.Path, "/ide/")
		handleIDEIcon(w, r, ideName)
		return
	}

	fileParam := r.URL.Query().Get("file")
	if fileParam != "" {
		handleFileIcon(w, r, fileParam)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/file/") {
		iconName := strings.TrimPrefix(r.URL.Path, "/file/")
		handleFileIcon(w, r, iconName)
		return
	}

	id := r.URL.Query().Get("value")
	if id == "" {
		id = path.Base(strings.TrimSuffix(r.URL.Path, "/"))
	}

	value, err := strconv.ParseFloat(id, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		http.Error(w, "percentage must be a number", http.StatusBadRequest)
		return
	}

	successColor, ok := parseOptionalColor(r.URL.Query().Get("successColor"))
	if !ok {
		http.Error(w, "successColor must be a 6-character hex value", http.StatusBadRequest)
		return
	}

	warningColor, ok := parseOptionalColor(r.URL.Query().Get("warningColor"))
	if !ok {
		http.Error(w, "warningColor must be a 6-character hex value", http.StatusBadRequest)
		return
	}

	dangerColor, ok := parseOptionalColor(r.URL.Query().Get("dangerColor"))
	if !ok {
		http.Error(w, "dangerColor must be a 6-character hex value", http.StatusBadRequest)
		return
	}

	barColor, ok := parseOptionalColor(r.URL.Query().Get("barColor"))
	if !ok {
		http.Error(w, "barColor must be a 6-character hex value", http.StatusBadRequest)
		return
	}

	customLabel := r.URL.Query().Get("label")
	if utf8.RuneCountInString(customLabel) > maxLabelRunes {
		http.Error(w, "label is too long (max 64 characters)", http.StatusBadRequest)
		return
	}

	minRaw := r.URL.Query().Get("min")
	maxRaw := r.URL.Query().Get("max")
	hasMin := minRaw != ""
	hasMax := maxRaw != ""

	if hasMin != hasMax {
		http.Error(w, "min and max must be provided together", http.StatusBadRequest)
		return
	}

	percentage := clampPercentage(value)
	if hasMin {
		minValue, minErr := strconv.ParseFloat(minRaw, 64)
		maxValue, maxErr := strconv.ParseFloat(maxRaw, 64)
		if minErr != nil || maxErr != nil || math.IsNaN(minValue) || math.IsNaN(maxValue) || math.IsInf(minValue, 0) || math.IsInf(maxValue, 0) {
			http.Error(w, "min and max must be numeric values", http.StatusBadRequest)
			return
		}

		if maxValue <= minValue {
			http.Error(w, "max must be greater than min", http.StatusBadRequest)
			return
		}

		normalized := ((value - minValue) / (maxValue - minValue)) * maxPercentage
		percentage = clampPercentage(normalized)
	}

	pickedColor := pickColor(percentage, successColor, warningColor, dangerColor)
	if barColor != "" {
		pickedColor = barColor
	}

	label := formatPercentLabel(percentage)
	if hasMin {
		label = formatNumber(value)
	}
	if customLabel != "" {
		label = customLabel
	}

	// Detached mode layout logic (?d)
	_, hasD := r.URL.Query()["d"]
	totalWidth := totalBarWidth
	textX := 45.0
	textAnchor := "middle"
	textColor := "#fff"

	if hasD {
		totalWidth = 145.0
		textX = 96.0
		textAnchor = "start"
		textColor = "#24292f"
	}

	hasCustomTextColor := false
	customTextColor, ok := parseOptionalColor(r.URL.Query().Get("textColor"))
	if ok && customTextColor != "" {
		textColor = customTextColor
		hasCustomTextColor = true
	}

	data := Data{
		BackgroundColor:    grey,
		Label:              label,
		Progress:           percentageToWidth(percentage),
		PickedColor:        pickedColor,
		Detached:           hasD,
		HasCustomTextColor: hasCustomTextColor,
		TotalWidth:         totalWidth,
		TextX:              textX,
		TextAnchor:         textAnchor,
		TextColor:          textColor,
	}

	buf := new(bytes.Buffer)
	err = progressTemplate.Execute(buf, data)
	if err != nil {
		http.Error(w, "failed to render SVG", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", cacheControlValue)

	if r.Method == http.MethodHead {
		return
	}

	_, _ = w.Write(buf.Bytes())
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ide/", func(w http.ResponseWriter, r *http.Request) {
		ideName := strings.TrimPrefix(r.URL.Path, "/ide/")
		handleIDEIcon(w, r, ideName)
	})
	mux.HandleFunc("/file/", func(w http.ResponseWriter, r *http.Request) {
		iconName := strings.TrimPrefix(r.URL.Path, "/file/")
		handleFileIcon(w, r, iconName)
	})
	mux.HandleFunc("/progress/", handler)
	mux.HandleFunc("/", handler)

	log.Printf("Listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("http.ListenAndServe: %v", err)
	}
}
