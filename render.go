package main

import (
	"math"
	"strings"
)

const RESET = "\x1b[0m"

type ThemeColors struct {
	// 스타일
	Dim  string
	Bold string

	// Semantic roles
	Model     string
	Folder    string
	Branch    string
	Safe      string
	Warning   string
	Danger    string
	Secondary string
	Accent    string
	Info      string

	// 프로그레스바
	BarFilled string
	BarEmpty  string

	// 기본 ANSI
	Red     string
	Green   string
	Yellow  string
	Blue    string
	Magenta string
	Cyan    string
	White   string
	Gray    string
}

var themes = map[string]ThemeColors{
	"default": {
		Dim: "\x1b[2m", Bold: "\x1b[1m",
		Model: "\x1b[38;5;117m", Folder: "\x1b[38;5;222m", Branch: "\x1b[38;5;218m",
		Safe: "\x1b[38;5;151m", Warning: "\x1b[38;5;222m", Danger: "\x1b[38;5;210m",
		Secondary: "\x1b[38;5;249m", Accent: "\x1b[38;5;222m", Info: "\x1b[38;5;117m",
		BarFilled: "\x1b[38;5;117m", BarEmpty: "\x1b[38;5;240m",
		Red: "\x1b[31m", Green: "\x1b[32m", Yellow: "\x1b[33m", Blue: "\x1b[34m",
		Magenta: "\x1b[35m", Cyan: "\x1b[36m", White: "\x1b[37m", Gray: "\x1b[90m",
	},
	"minimal": {
		Dim: "\x1b[2m", Bold: "\x1b[1m",
		Model: "\x1b[37m", Folder: "\x1b[37m", Branch: "\x1b[37m",
		Safe: "\x1b[90m", Warning: "\x1b[37m", Danger: "\x1b[1;37m",
		Secondary: "\x1b[90m", Accent: "\x1b[37m", Info: "\x1b[37m",
		BarFilled: "\x1b[37m", BarEmpty: "\x1b[90m",
		Red: "\x1b[31m", Green: "\x1b[32m", Yellow: "\x1b[33m", Blue: "\x1b[34m",
		Magenta: "\x1b[35m", Cyan: "\x1b[36m", White: "\x1b[37m", Gray: "\x1b[90m",
	},
	"catppuccin": {
		Dim: "\x1b[2m", Bold: "\x1b[1m",
		Model: "\x1b[38;2;137;180;250m", Folder: "\x1b[38;2;249;226;175m", Branch: "\x1b[38;2;245;194;231m",
		Safe: "\x1b[38;2;166;227;161m", Warning: "\x1b[38;2;250;179;135m", Danger: "\x1b[38;2;243;139;168m",
		Secondary: "\x1b[38;2;127;132;156m", Accent: "\x1b[38;2;249;226;175m", Info: "\x1b[38;2;137;180;250m",
		BarFilled: "\x1b[38;2;137;180;250m", BarEmpty: "\x1b[38;2;69;71;90m",
		Red: "\x1b[31m", Green: "\x1b[32m", Yellow: "\x1b[33m", Blue: "\x1b[34m",
		Magenta: "\x1b[35m", Cyan: "\x1b[36m", White: "\x1b[37m", Gray: "\x1b[90m",
	},
	"dracula": {
		Dim: "\x1b[2m", Bold: "\x1b[1m",
		Model: "\x1b[38;2;189;147;249m", Folder: "\x1b[38;2;255;184;108m", Branch: "\x1b[38;2;255;121;198m",
		Safe: "\x1b[38;2;80;250;123m", Warning: "\x1b[38;2;241;250;140m", Danger: "\x1b[38;2;255;85;85m",
		Secondary: "\x1b[38;2;98;114;164m", Accent: "\x1b[38;2;255;184;108m", Info: "\x1b[38;2;189;147;249m",
		BarFilled: "\x1b[38;2;189;147;249m", BarEmpty: "\x1b[38;2;68;71;90m",
		Red: "\x1b[31m", Green: "\x1b[32m", Yellow: "\x1b[33m", Blue: "\x1b[34m",
		Magenta: "\x1b[35m", Cyan: "\x1b[36m", White: "\x1b[37m", Gray: "\x1b[90m",
	},
	"gruvbox": {
		Dim: "\x1b[2m", Bold: "\x1b[1m",
		Model: "\x1b[38;2;215;153;33m", Folder: "\x1b[38;2;250;189;47m", Branch: "\x1b[38;2;211;134;155m",
		Safe: "\x1b[38;2;184;187;38m", Warning: "\x1b[38;2;250;189;47m", Danger: "\x1b[38;2;204;36;29m",
		Secondary: "\x1b[38;2;168;153;132m", Accent: "\x1b[38;2;250;189;47m", Info: "\x1b[38;2;215;153;33m",
		BarFilled: "\x1b[38;2;215;153;33m", BarEmpty: "\x1b[38;2;80;73;69m",
		Red: "\x1b[31m", Green: "\x1b[32m", Yellow: "\x1b[33m", Blue: "\x1b[34m",
		Magenta: "\x1b[35m", Cyan: "\x1b[36m", White: "\x1b[37m", Gray: "\x1b[90m",
	},
	"nord": {
		Dim: "\x1b[2m", Bold: "\x1b[1m",
		Model: "\x1b[38;2;136;192;208m", Folder: "\x1b[38;2;235;203;139m", Branch: "\x1b[38;2;180;142;173m",
		Safe: "\x1b[38;2;163;190;140m", Warning: "\x1b[38;2;235;203;139m", Danger: "\x1b[38;2;191;97;106m",
		Secondary: "\x1b[38;2;76;86;106m", Accent: "\x1b[38;2;235;203;139m", Info: "\x1b[38;2;136;192;208m",
		BarFilled: "\x1b[38;2;136;192;208m", BarEmpty: "\x1b[38;2;59;66;82m",
		Red: "\x1b[31m", Green: "\x1b[32m", Yellow: "\x1b[33m", Blue: "\x1b[34m",
		Magenta: "\x1b[35m", Cyan: "\x1b[36m", White: "\x1b[37m", Gray: "\x1b[90m",
	},
	"tokyoNight": {
		Dim: "\x1b[2m", Bold: "\x1b[1m",
		Model: "\x1b[38;2;122;162;247m", Folder: "\x1b[38;2;224;175;104m", Branch: "\x1b[38;2;187;154;247m",
		Safe: "\x1b[38;2;158;206;106m", Warning: "\x1b[38;2;224;175;104m", Danger: "\x1b[38;2;247;118;142m",
		Secondary: "\x1b[38;2;86;95;137m", Accent: "\x1b[38;2;224;175;104m", Info: "\x1b[38;2;122;162;247m",
		BarFilled: "\x1b[38;2;122;162;247m", BarEmpty: "\x1b[38;2;59;66;82m",
		Red: "\x1b[31m", Green: "\x1b[32m", Yellow: "\x1b[33m", Blue: "\x1b[34m",
		Magenta: "\x1b[35m", Cyan: "\x1b[36m", White: "\x1b[37m", Gray: "\x1b[90m",
	},
	"solarized": {
		Dim: "\x1b[2m", Bold: "\x1b[1m",
		Model: "\x1b[38;2;38;139;210m", Folder: "\x1b[38;2;181;137;0m", Branch: "\x1b[38;2;211;54;130m",
		Safe: "\x1b[38;2;133;153;0m", Warning: "\x1b[38;2;181;137;0m", Danger: "\x1b[38;2;220;50;47m",
		Secondary: "\x1b[38;2;88;110;117m", Accent: "\x1b[38;2;181;137;0m", Info: "\x1b[38;2;38;139;210m",
		BarFilled: "\x1b[38;2;38;139;210m", BarEmpty: "\x1b[38;2;88;110;117m",
		Red: "\x1b[31m", Green: "\x1b[32m", Yellow: "\x1b[33m", Blue: "\x1b[34m",
		Magenta: "\x1b[35m", Cyan: "\x1b[36m", White: "\x1b[37m", Gray: "\x1b[90m",
	},
}

func getTheme(name string) ThemeColors {
	if t, ok := themes[name]; ok {
		return t
	}
	return themes["default"]
}

func getColorForPercent(percent int, theme ThemeColors) string {
	if percent <= 50 {
		return theme.Safe
	}
	if percent <= 80 {
		return theme.Warning
	}
	return theme.Danger
}

func renderSeparator(style string, theme ThemeColors) string {
	switch style {
	case "space":
		return "  "
	case "dot":
		return " " + theme.Dim + "·" + RESET + " "
	case "arrow":
		return " " + theme.Dim + "›" + RESET + " "
	default: // "pipe"
		return " " + theme.Dim + "│" + RESET + " "
	}
}

func renderProgressBar(percent int, theme ThemeColors) string {
	const width = 10
	filled := int(math.Round(float64(percent) / 100.0 * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	empty := width - filled
	return getColorForPercent(percent, theme) +
		strings.Repeat("█", filled) +
		theme.BarEmpty + strings.Repeat("░", empty) + RESET
}
