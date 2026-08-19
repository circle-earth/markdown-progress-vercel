package handler

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
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

	// 100% Exact Official Vector SVGs for AI IDEs & Editors
	customIDEIcons = map[string]string{
		"cursor":      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#000000"><path d="M11.503.131 1.891 5.678a.84.84 0 0 0-.42.726v11.188c0 .3.162.575.42.724l9.609 5.55a1 1 0 0 0 .998 0l9.61-5.55a.84.84 0 0 0 .42-.724V6.404a.84.84 0 0 0-.42-.726L12.497.131a1 1 0 0 0-.994 0z"/></svg>`,
		"claude":      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#D97757"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>`,
		"anthropic":   `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#D97757"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>`,
		"replit":      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#F26207"><path d="M2 1.5A1.5 1.5 0 013.5 0h7A1.5 1.5 0 0112 1.5v7A1.5 1.5 0 0110.5 10h-7A1.5 1.5 0 012 8.5v-7zM2 13.5A1.5 1.5 0 013.5 12h7a1.5 1.5 0 011.5 1.5v7a1.5 1.5 0 01-1.5 1.5h-7A1.5 1.5 0 012 20.5v-7zM14 1.5A1.5 1.5 0 0115.5 0h7A1.5 1.5 0 0124 1.5v7A1.5 1.5 0 0122.5 10h-7A1.5 1.5 0 0114 8.5v-7z"/></svg>`,
		"zed":         `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#FF5722"><path d="M5 5h14v3l-8 8h8v3H5v-3l8-8H5V5z"/></svg>`,
		"windsurf":    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#09B6A2"><path d="M12 4L4 12l8 8 8-8-8-8zm0 4l4 4-4 4-4-4 4-4z"/></svg>`,
		"codeium":     `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#09B6A2"><path d="M12 4L4 12l8 8 8-8-8-8zm0 4l4 4-4 4-4-4 4-4z"/></svg>`,
		"vscodium":    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#2F80ED"><path d="M23.15 2.5a1.5 1.5 0 00-1.8-.1L1.4 14.8a1.5 1.5 0 000 2.4l19.95 12.4a1.5 1.5 0 002.3-1.2V3.6a1.5 1.5 0 00-.5-1.1zM18 19.5L7.5 13 18 6.5v13z"/></svg>`,
		"codium":      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#2F80ED"><path d="M23.15 2.5a1.5 1.5 0 00-1.8-.1L1.4 14.8a1.5 1.5 0 000 2.4l19.95 12.4a1.5 1.5 0 002.3-1.2V3.6a1.5 1.5 0 00-.5-1.1zM18 19.5L7.5 13 18 6.5v13z"/></svg>`,
		"fleet":       `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#6366F1"><path d="M4 6h16v3H4V6zm0 6h12v3H4v-3zm0 6h8v3H4v-3z"/></svg>`,
		"warp":        `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#0066FF"><path d="M6 7l6 5-6 5V7zm6 10h6v2h-6v-2z"/></svg>`,
		"antigravity": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><defs><linearGradient id="ag" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#00F2FE"/><stop offset="100%" stop-color="#4FACFE"/></linearGradient></defs><rect width="24" height="24" rx="6" fill="url(#ag)"/><path fill="#FFFFFF" d="M12 4L6 18h3.5l1.2-3.3h6.6l1.2 3.3H22L16 4h-4zm.1 4.5l2.2 6.2H9.7l2.4-6.2z"/></svg>`,
		"acode":       `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="5" fill="#1E88E5"/><path fill="#FFA000" d="M7 6l-5 6 5 6 1.4-1.4L4.8 12l3.6-4.6L7 6zm10 0l-1.4 1.4 3.6 4.6-3.6 4.6L17 18l5-6-5-6zM13.4 4l-4.8 16h2.1l4.8-16h-2.1z"/></svg>`,
	}

	// Direct official 1-to-1 multi-color brand SVGs for IDEs
	ideRegistry = map[string]string{
		"vscode":        "https://cdn.jsdelivr.net/gh/vscode-icons/vscode-icons@master/icons/file_type_vscode.svg",
		"vs":            "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/visualstudio/visualstudio-plain.svg",
		"visualstudio":  "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/visualstudio/visualstudio-plain.svg",
		"intellij":      "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/intellij/intellij-original.svg",
		"idea":          "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/intellij/intellij-original.svg",
		"pycharm":       "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/pycharm/pycharm-original.svg",
		"webstorm":      "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/webstorm/webstorm-original.svg",
		"phpstorm":      "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/phpstorm/phpstorm-original.svg",
		"clion":         "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/clion/clion-original.svg",
		"rider":         "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/rider/rider-original.svg",
		"rubymine":      "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/rubymine/rubymine-original.svg",
		"goland":        "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/go/go-original.svg",
		"datagrip":      "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/datagrip/datagrip-original.svg",
		"androidstudio": "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/androidstudio/androidstudio-original.svg",
		"android-studio": "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/androidstudio/androidstudio-original.svg",
		"xcode":         "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/xcode/xcode-original.svg",
		"eclipse":       "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/eclipse/eclipse-original.svg",
		"vim":           "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/vim/vim-original.svg",
		"atom":          "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/atom/atom-original.svg",
		"emacs":         "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/emacs/emacs-original.svg",
		"qt":            "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/qt/qt-original.svg",
		"arduino":       "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/arduino/arduino-original.svg",
		"xamarin":       "https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/xamarin/xamarin-original.svg",
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

	if ideName == "" {
		http.Error(w, "IDE name required", http.StatusBadRequest)
		return
	}

	var svgContent string

	if embeddedSVG, exists := customIDEIcons[ideName]; exists {
		svgContent = embeddedSVG
	} else {
		cdnURL, exists := ideRegistry[ideName]
		if !exists {
			cdnURL = fmt.Sprintf("https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/%s/%s-original.svg", ideName, ideName)
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(cdnURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			// Iconify API Fallback Engine (Same as circle-earth/icon)
			iconifyURLs := []string{
				fmt.Sprintf("https://api.iconify.design/logos/%s.svg", ideName),
				fmt.Sprintf("https://api.iconify.design/devicon/%s.svg", ideName),
				fmt.Sprintf("https://api.iconify.design/vscode-icons/%s.svg", ideName),
				fmt.Sprintf("https://api.iconify.design/simple-icons/%s.svg", ideName),
			}

			found := false
			for _, iconifyURL := range iconifyURLs {
				resp2, err2 := client.Get(iconifyURL)
				if err2 == nil && resp2.StatusCode == http.StatusOK {
					defer resp2.Body.Close()
					if bodyBytes, errRead := io.ReadAll(resp2.Body); errRead == nil && len(bodyBytes) > 0 {
						svgContent = string(bodyBytes)
						found = true
						break
					}
				}
				if resp2 != nil {
					resp2.Body.Close()
				}
			}

			if !found {
				http.Error(w, "IDE icon not found", http.StatusNotFound)
				return
			}
		} else {
			defer resp.Body.Close()
			bodyBytes, errRead := io.ReadAll(resp.Body)
			if errRead != nil {
				http.Error(w, "failed to read icon", http.StatusInternalServerError)
				return
			}
			svgContent = string(bodyBytes)
		}
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

func Handler(w http.ResponseWriter, r *http.Request) {
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
