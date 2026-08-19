package main

import (
	"bytes"
	"embed"
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

//go:embed svg/ide/*.svg
var ideSVGFiles embed.FS

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
	IconSVG            string
	HasIcon            bool
}

const (
	minPercentage     = 0.0
	maxPercentage     = 100.0
	totalBarWidth     = 90.0
	cacheControlValue = "public, max-age=300, s-maxage=300"
	maxLabelRunes     = 64

	svgTemplate = `<svg width="{{.TotalWidth}}" height="20" viewBox="0 0 {{.TotalWidth}} 20" xmlns="http://www.w3.org/2000/svg">
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
  {{if .HasIcon}}
  {{.IconSVG}}
  {{end}}
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
	viewBoxRegex   = regexp.MustCompile(`(?i)viewBox="[^"]*"`)

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

	displayNameMap = map[string]string{
		"vscode":             "VS Code",
		"visual-studio-code": "VS Code",
		"cursor":             "Cursor",
		"claude":             "Claude",
		"anthropic":          "Claude",
		"claudecode":         "Claude Code",
		"antigravity":        "Antigravity",
		"acode":              "Acode",
		"intellij":           "IntelliJ IDEA",
		"idea":               "IntelliJ IDEA",
		"pycharm":            "PyCharm",
		"webstorm":           "WebStorm",
		"phpstorm":           "PhpStorm",
		"clion":              "CLion",
		"rider":              "Rider",
		"rubymine":           "RubyMine",
		"goland":             "GoLand",
		"datagrip":           "DataGrip",
		"fleet":              "Fleet",
		"androidstudio":      "Android Studio",
		"android-studio":     "Android Studio",
		"xcode":              "Xcode",
		"vim":                "Vim",
		"neovim":             "Neovim",
		"emacs":              "Emacs",
		"sublime":            "Sublime Text",
		"sublimetext":        "Sublime Text",
		"atom":               "Atom",
		"replit":             "Replit",
		"zed":                "Zed",
		"windsurf":           "Windsurf",
		"codeium":            "Codeium",
		"vscodium":           "VSCodium",
		"warp":               "Warp",
		"visualstudio":       "Visual Studio",
		"vs":                 "Visual Studio",
		"eclipse":            "Eclipse",
		"qt":                 "Qt Creator",
		"arduino":            "Arduino",
		"xamarin":            "Xamarin",
		"terminal":           "Terminal",
		"word":               "Word",
		"excel":              "Excel",
		"powerpoint":         "PowerPoint",
		"edge":               "Edge",
		"textmate":           "TextMate",
		"nova":               "Nova",
		"wpsoffice":          "WPS Office",
		"node":               "Node.js",
		"nodejs":             "Node.js",
		"python":             "Python",
		"py":                 "Python",
		"go":                 "Go",
		"golang":             "Go",
		"typescript":         "TypeScript",
		"ts":                 "TypeScript",
		"javascript":         "JavaScript",
		"js":                 "JavaScript",
		"react":              "React",
		"reactjs":            "React",
		"rust":               "Rust",
		"rs":                 "Rust",
		"docker":             "Docker",
		"git":                "Git",
		"json":               "JSON",
		"html":               "HTML5",
		"css":                "CSS3",
	}

	brandColorMap = map[string]string{
		"vscode":             "#007ACC",
		"visual-studio-code": "#007ACC",
		"cursor":             "#1E1E1E",
		"claude":             "#D97757",
		"anthropic":          "#D97757",
		"claudecode":         "#D97757",
		"antigravity":        "#00F2FE",
		"acode":              "#1E88E5",
		"intellij":           "#FE2857",
		"idea":               "#FE2857",
		"pycharm":            "#21D789",
		"webstorm":           "#00CDD7",
		"phpstorm":           "#B052C0",
		"clion":              "#21D789",
		"rider":              "#E21245",
		"rubymine":           "#FE2857",
		"goland":             "#00ADD8",
		"datagrip":           "#21D789",
		"fleet":              "#6366F1",
		"androidstudio":      "#3DDC84",
		"android-studio":     "#3DDC84",
		"xcode":              "#147EFB",
		"vim":                "#019833",
		"neovim":             "#57A143",
		"emacs":              "#7F5AB6",
		"sublime":            "#FF9800",
		"sublimetext":        "#FF9800",
		"atom":               "#66595C",
		"replit":             "#F26207",
		"zed":                "#FF5722",
		"windsurf":           "#09B6A2",
		"codeium":            "#09B6A2",
		"vscodium":           "#2F80ED",
		"warp":               "#0066FF",
		"visualstudio":       "#5C2D91",
		"vs":                 "#5C2D91",
		"eclipse":            "#2C2255",
		"qt":                 "#41CD52",
		"arduino":            "#00979D",
		"xamarin":            "#3498DB",
		"terminal":           "#1E1E1E",
		"word":               "#185ABD",
		"excel":              "#107C41",
		"powerpoint":         "#C43E1C",
		"edge":               "#0078D7",
		"textmate":           "#212121",
		"nova":               "#8E44AD",
		"wpsoffice":          "#FF334B",
		"node":               "#339933",
		"nodejs":             "#339933",
		"python":             "#3776AB",
		"py":                 "#3776AB",
		"go":                 "#00ADD8",
		"golang":             "#00ADD8",
		"typescript":         "#3178C6",
		"ts":                 "#3178C6",
		"javascript":         "#F7DF1E",
		"js":                 "#F7DF1E",
		"react":              "#61DAFB",
		"reactjs":            "#61DAFB",
		"rust":               "#333333",
		"rs":                 "#333333",
		"docker":             "#2496ED",
		"git":                "#F05032",
		"json":               "#292929",
		"html":               "#E34F26",
		"css":                "#1572B6",
	}
)

func getDisplayName(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	clean = strings.TrimSuffix(clean, ".svg")
	if title, exists := displayNameMap[clean]; exists {
		return title
	}
	parts := strings.Split(clean, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func formatIconSVG(rawSVG string) string {
	viewBoxAttr := `viewBox="0 0 24 24"`
	if match := viewBoxRegex.FindString(rawSVG); match != "" {
		viewBoxAttr = match
	}
	svgTagStart := strings.Index(rawSVG, ">")
	svgTagEnd := strings.LastIndex(rawSVG, "</svg>")
	innerContent := rawSVG
	if svgTagStart != -1 && svgTagEnd != -1 && svgTagEnd > svgTagStart {
		innerContent = rawSVG[svgTagStart+1 : svgTagEnd]
	}
	// Center 16x16 icon inside left bar at x=37, y=2
	return fmt.Sprintf(`<svg x="37" y="2" width="16" height="16" %s preserveAspectRatio="xMidYMid meet">%s</svg>`, viewBoxAttr, innerContent)
}

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
	return 24 // Default size if omitted
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

	// 100% In-Memory Embedded SVG lookup from svg/ide/*.svg
	filePath := fmt.Sprintf("svg/ide/%s.svg", ideName)
	svgBytes, err := ideSVGFiles.ReadFile(filePath)
	if err != nil {
		filePath = fmt.Sprintf("svg/ide/%s.svg", cleanName)
		svgBytes, err = ideSVGFiles.ReadFile(filePath)
	}
	if err != nil {
		// Fallback to default.svg
		svgBytes, _ = ideSVGFiles.ReadFile("svg/ide/default.svg")
	}

	rawSVG := string(svgBytes)
	_, isRaw := r.URL.Query()["raw"]
	if isRaw {
		sizeVal := getRequestedSize(r)
		finalSVG := setSVGSize(rawSVG, sizeVal)
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", cacheControlValue)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(finalSVG))
		return
	}

	displayName := getDisplayName(ideName)
	if customLabel := r.URL.Query().Get("label"); customLabel != "" {
		displayName = customLabel
	}

	barColor := "#333"
	if bc, ok := brandColorMap[cleanName]; ok {
		barColor = bc
	}
	if customBg := r.URL.Query().Get("bg"); customBg != "" {
		barColor = customBg
	} else if customBg := r.URL.Query().Get("color"); customBg != "" {
		barColor = customBg
	}

	textColor := "#24292f"
	hasCustomTextColor := false
	if customTextColor, ok := parseOptionalColor(r.URL.Query().Get("textColor")); ok && customTextColor != "" {
		textColor = customTextColor
		hasCustomTextColor = true
	}

	// Pass exact Data struct to progressTemplate (100% unified with progress bar template)
	data := Data{
		BackgroundColor:    grey,
		Label:              displayName,
		Progress:           90, // 100% full colored bar (90px)
		PickedColor:        barColor,
		Detached:           true,
		HasCustomTextColor: hasCustomTextColor,
		TotalWidth:         145.0,
		TextX:              96.0,
		TextAnchor:         "start",
		TextColor:          textColor,
		IconSVG:            formatIconSVG(rawSVG),
		HasIcon:            true,
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

	_, _ = w.Write([]byte(buf.Bytes()))
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

	rawSVG := string(bodyBytes)
	_, isRaw := r.URL.Query()["raw"]
	if isRaw {
		sizeVal := getRequestedSize(r)
		finalSVG := setSVGSize(rawSVG, sizeVal)
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", cacheControlValue)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(finalSVG))
		return
	}

	displayName := getDisplayName(iconName)
	if customLabel := r.URL.Query().Get("label"); customLabel != "" {
		displayName = customLabel
	}

	barColor := "#333"
	if bc, ok := brandColorMap[iconName]; ok {
		barColor = bc
	}
	if customBg := r.URL.Query().Get("bg"); customBg != "" {
		barColor = customBg
	} else if customBg := r.URL.Query().Get("color"); customBg != "" {
		barColor = customBg
	}

	textColor := "#24292f"
	hasCustomTextColor := false
	if customTextColor, ok := parseOptionalColor(r.URL.Query().Get("textColor")); ok && customTextColor != "" {
		textColor = customTextColor
		hasCustomTextColor = true
	}

	// Pass exact Data struct to progressTemplate (100% unified with progress bar template)
	data := Data{
		BackgroundColor:    grey,
		Label:              displayName,
		Progress:           90, // 100% full colored bar (90px)
		PickedColor:        barColor,
		Detached:           true,
		HasCustomTextColor: hasCustomTextColor,
		TotalWidth:         145.0,
		TextX:              96.0,
		TextAnchor:         "start",
		TextColor:          textColor,
		IconSVG:            formatIconSVG(rawSVG),
		HasIcon:            true,
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

	_, _ = w.Write([]byte(buf.Bytes()))
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
