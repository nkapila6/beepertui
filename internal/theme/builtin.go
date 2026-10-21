package theme

// Builtins are the terminal palettes these themes publish for their own ports
// (kitty, Alacritty and the like), so they match what people know.
var Builtins = map[string]Theme{
	"catppuccin-mocha": {Bg: "#1e1e2e", Fg: "#cdd6f4", Colors: [16]string{
		"#45475a", "#f38ba8", "#a6e3a1", "#f9e2af", "#89b4fa", "#f5c2e7", "#94e2d5", "#bac2de",
		"#585b70", "#f38ba8", "#a6e3a1", "#f9e2af", "#89b4fa", "#f5c2e7", "#94e2d5", "#a6adc8"}},
	"catppuccin-latte": {Bg: "#eff1f5", Fg: "#4c4f69", Colors: [16]string{
		"#5c5f77", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#acb0be",
		"#6c6f85", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#bcc0cc"}},
	"tokyonight": {Bg: "#1a1b26", Fg: "#c0caf5", Colors: [16]string{
		"#15161e", "#f7768e", "#9ece6a", "#e0af68", "#7aa2f7", "#bb9af7", "#7dcfff", "#a9b1d6",
		"#414868", "#f7768e", "#9ece6a", "#e0af68", "#7aa2f7", "#bb9af7", "#7dcfff", "#c0caf5"}},
	"gruvbox": {Bg: "#282828", Fg: "#ebdbb2", Colors: [16]string{
		"#282828", "#cc241d", "#98971a", "#d79921", "#458588", "#b16286", "#689d6a", "#a89984",
		"#928374", "#fb4934", "#b8bb26", "#fabd2f", "#83a598", "#d3869b", "#8ec07c", "#ebdbb2"}},
	"gruvbox-light": {Bg: "#fbf1c7", Fg: "#3c3836", Colors: [16]string{
		"#fbf1c7", "#cc241d", "#98971a", "#d79921", "#458588", "#b16286", "#689d6a", "#7c6f64",
		"#928374", "#9d0006", "#79740e", "#b57614", "#076678", "#8f3f71", "#427b58", "#3c3836"}},
	"nord": {Bg: "#2e3440", Fg: "#d8dee9", Colors: [16]string{
		"#3b4252", "#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead", "#88c0d0", "#e5e9f0",
		"#4c566a", "#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead", "#8fbcbb", "#eceff4"}},
	"rose-pine": {Bg: "#191724", Fg: "#e0def4", Colors: [16]string{
		"#26233a", "#eb6f92", "#31748f", "#f6c177", "#9ccfd8", "#c4a7e7", "#ebbcba", "#e0def4",
		"#6e6a86", "#eb6f92", "#31748f", "#f6c177", "#9ccfd8", "#c4a7e7", "#ebbcba", "#e0def4"}},
	"kanagawa": {Bg: "#1f1f28", Fg: "#dcd7ba", Colors: [16]string{
		"#16161d", "#c34043", "#76946a", "#c0a36e", "#7e9cd8", "#957fb8", "#6a9589", "#c8c093",
		"#727169", "#e82424", "#98bb6c", "#e6c384", "#7fb4ca", "#938aa9", "#7aa89f", "#dcd7ba"}},
	"dracula": {Bg: "#282a36", Fg: "#f8f8f2", Colors: [16]string{
		"#21222c", "#ff5555", "#50fa7b", "#f1fa8c", "#bd93f9", "#ff79c6", "#8be9fd", "#f8f8f2",
		"#6272a4", "#ff6e6e", "#69ff94", "#ffffa5", "#d6acff", "#ff92df", "#a4ffff", "#ffffff"}},
	"everforest": {Bg: "#2d353b", Fg: "#d3c6aa", Colors: [16]string{
		"#343f44", "#e67e80", "#a7c080", "#dbbc7f", "#7fbbb3", "#d699b6", "#83c092", "#d3c6aa",
		"#859289", "#e67e80", "#a7c080", "#dbbc7f", "#7fbbb3", "#d699b6", "#83c092", "#d3c6aa"}},
	"one-dark": {Bg: "#282c34", Fg: "#abb2bf", Colors: [16]string{
		"#282c34", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#abb2bf",
		"#5c6370", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#ffffff"}},
	"solarized-dark": {Bg: "#002b36", Fg: "#839496", Colors: [16]string{
		"#073642", "#dc322f", "#859900", "#b58900", "#268bd2", "#d33682", "#2aa198", "#eee8d5",
		"#586e75", "#cb4b16", "#859900", "#b58900", "#268bd2", "#6c71c4", "#2aa198", "#fdf6e3"}},
}
