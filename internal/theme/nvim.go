package theme

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Neovim runs headless with your own config, so plugin colorschemes load. The
// Lua reads names from the environment and writes JSON to a file, so a theme
// name never ends up inside code, and whatever your config prints doesn't
// get mixed into the result.

const nvimRead = `
local ok = pcall(vim.cmd.colorscheme, os.getenv("BEEPERTUI_THEME"))
local function hl(name, attr)
  local h = vim.api.nvim_get_hl(0, { name = name, link = false })
  return h[attr] and string.format("#%06x", h[attr]) or nil
end
local out = { ok = ok, dark = vim.o.background == "dark", colors = {}, fb = {
  bg = hl("Normal", "bg"), fg = hl("Normal", "fg"),
  red = hl("ErrorMsg", "fg") or hl("Error", "fg") or hl("DiagnosticError", "fg"),
  green = hl("String", "fg"),
  yellow = hl("WarningMsg", "fg") or hl("Type", "fg") or hl("DiagnosticWarn", "fg"),
  blue = hl("Function", "fg"),
  magenta = hl("Keyword", "fg") or hl("Statement", "fg"),
  cyan = hl("Special", "fg") or hl("Constant", "fg"),
  comment = hl("Comment", "fg"),
  float = hl("NormalFloat", "bg") or hl("Pmenu", "bg"),
} }
for i = 0, 15 do out.colors[i + 1] = vim.g["terminal_color_" .. i] or "" end
local f = io.open(os.getenv("BEEPERTUI_OUT"), "w")
f:write(vim.json.encode(out))
f:close()
`

const nvimList = `
local f = io.open(os.getenv("BEEPERTUI_OUT"), "w")
f:write(table.concat(vim.fn.getcompletion("", "color"), "\n"))
f:close()
`

func runNvim(ctx context.Context, lua string, env ...string) ([]byte, error) {
	nvim, err := exec.LookPath("nvim")
	if err != nil {
		return nil, fmt.Errorf("nvim isn't installed")
	}
	out, err := os.CreateTemp("", "beepertui-nvim-*")
	if err != nil {
		return nil, err
	}
	out.Close()
	defer os.Remove(out.Name())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, nvim, "--headless", "-c", "lua "+strings.ReplaceAll(lua, "\n", " "), "-c", "qa!")
	cmd.Env = append(os.Environ(), append(env, "BEEPERTUI_OUT="+out.Name())...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("nvim: %v %s", err, strings.TrimSpace(string(b)))
	}
	return os.ReadFile(out.Name())
}

// NvimColorschemes lists every colorscheme your Neovim can load.
func NvimColorschemes(ctx context.Context) ([]string, error) {
	b, err := runNvim(ctx, nvimList)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range strings.Split(string(b), "\n") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

// ImportNvim loads a Neovim colorscheme and saves it as a theme file. Most
// themes set terminal colours; for the ones that don't, the palette comes
// from highlight groups (strings green, functions blue, ...).
func ImportNvim(ctx context.Context, configDir, scheme string) (Theme, error) {
	b, err := runNvim(ctx, nvimRead, "BEEPERTUI_THEME="+scheme)
	if err != nil {
		return Theme{}, err
	}
	var r struct {
		OK     bool              `json:"ok"`
		Dark   bool              `json:"dark"`
		Colors []string          `json:"colors"`
		FB     map[string]string `json:"fb"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return Theme{}, fmt.Errorf("reading nvim's answer: %w", err)
	}
	if !r.OK {
		return Theme{}, fmt.Errorf("nvim has no colorscheme %q", scheme)
	}
	t := Theme{Name: "nvim:" + scheme}
	for i := 0; i < 16 && i < len(r.Colors); i++ {
		if isHex(r.Colors[i]) {
			t.Colors[i] = r.Colors[i]
		}
	}
	fb := r.FB
	t.Bg, t.Fg = fb["bg"], fb["fg"]
	if t.Bg == "" { // transparent themes leave Normal's background empty
		t.Bg = firstHex(fb["float"], map[bool]string{true: "#1e1e1e", false: "#fafafa"}[r.Dark])
	}
	if t.Fg == "" {
		t.Fg = map[bool]string{true: "#d0d0d0", false: "#303030"}[r.Dark]
	}
	fill := []struct {
		i   int
		key string
	}{{1, "red"}, {2, "green"}, {3, "yellow"}, {4, "blue"}, {5, "magenta"}, {6, "cyan"}}
	for _, f := range fill {
		if t.Colors[f.i] == "" && t.Colors[f.i+8] == "" {
			t.Colors[f.i] = firstHex(fb[f.key], t.Fg)
		}
	}
	if t.Colors[0] == "" && t.Colors[8] == "" {
		t.Colors[0] = firstHex(fb["float"], t.Bg)
		t.Colors[8] = firstHex(fb["comment"], t.Fg)
	}
	if t.Colors[7] == "" && t.Colors[15] == "" {
		t.Colors[7] = t.Fg
	}
	if err := t.complete(); err != nil {
		return Theme{}, err
	}
	return t, Save(configDir, t.Name, t)
}

func firstHex(vals ...string) string {
	for _, v := range vals {
		if isHex(v) {
			return v
		}
	}
	return ""
}
