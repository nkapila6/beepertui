// Package auth gets an access token from Beeper Desktop with the OAuth
// authorization code + PKCE flow its local API offers, and keeps it in the
// macOS Keychain (or a private file elsewhere).
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const service = "beepertui"

// Login runs the browser approval flow and returns a fresh access token.
// say gets short progress lines for the terminal.
func Login(ctx context.Context, baseURL string, say func(string)) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	redirect := fmt.Sprintf("http://%s/callback", ln.Addr())

	clientID, err := register(ctx, baseURL, redirect)
	if err != nil {
		return "", fmt.Errorf("registering with Beeper Desktop: %w", err)
	}

	verifier := randomString(48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randomString(24)

	authURL := baseURL + "/oauth/authorize?" + url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"scope":                 {"read write"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	type result struct {
		code string
		err  error
	}
	got := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		switch {
		case q.Get("state") != state:
			got <- result{err: errors.New("state mismatch, ignoring the callback")}
		case q.Get("error") != "":
			got <- result{err: fmt.Errorf("beeper said no: %s %s", q.Get("error"), q.Get("error_description"))}
		case q.Get("code") == "":
			got <- result{err: errors.New("callback had no code")}
		default:
			got <- result{code: q.Get("code")}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>beepertui</title>`+
			`<body style="font-family:system-ui;padding:3rem">beepertui is connected. You can close this tab.</body>`)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	say("Opening Beeper Desktop's approval page in your browser.")
	say("If it doesn't open, go to:\n  " + authURL)
	openBrowser(authURL)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var res result
	select {
	case res = <-got:
	case <-ctx.Done():
		return "", errors.New("timed out waiting for approval")
	}
	if res.err != nil {
		return "", res.err
	}
	return exchange(ctx, baseURL, clientID, res.code, verifier)
}

func register(ctx context.Context, baseURL, redirect string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"client_name":                "beepertui",
		"client_uri":                 "https://github.com/nkapila6/beepertui",
		"redirect_uris":              []string{redirect},
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      "read write",
	})
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := post(ctx, baseURL+"/oauth/register", "application/json", bytes.NewReader(body), &out); err != nil {
		return "", err
	}
	if out.ClientID == "" {
		return "", errors.New("no client_id in response")
	}
	return out.ClientID, nil
}

func exchange(ctx context.Context, baseURL, clientID, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"client_id":     {clientID},
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := post(ctx, baseURL+"/oauth/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), &out); err != nil {
		return "", fmt.Errorf("getting the token: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("no access_token in response")
	}
	return out.AccessToken, nil
}

// Revoke tells Beeper the token is done with. Best effort.
func Revoke(ctx context.Context, baseURL, token string) {
	form := url.Values{"token": {token}}
	post(ctx, baseURL+"/oauth/revoke", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), nil)
}

func post(ctx context.Context, u, ctype string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", ctype)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	cmd.Start()
}

// storage: Keychain on macOS, a 0600 file in the user config dir elsewhere

func tokenFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "beepertui", "token"), nil
}

// Save stores the token.
func Save(token string) error {
	if runtime.GOOS == "darwin" {
		// -U updates an existing item instead of failing
		return exec.Command("security", "add-generic-password", "-U", "-a", service, "-s", service, "-l", "beepertui access token", "-w", token).Run()
	}
	p, err := tokenFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(token+"\n"), 0o600)
}

// Load returns the stored token, or "" if there isn't one.
func Load() string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("security", "find-generic-password", "-a", service, "-s", service, "-w").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	p, err := tokenFile()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Delete removes the stored token.
func Delete() error {
	if runtime.GOOS == "darwin" {
		err := exec.Command("security", "delete-generic-password", "-a", service, "-s", service).Run()
		if Load() == "" {
			return nil
		}
		return err
	}
	p, err := tokenFile()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Where says where the token lives, for messages.
func Where() string {
	if runtime.GOOS == "darwin" {
		return "the macOS Keychain (item \"beepertui\")"
	}
	p, _ := tokenFile()
	return p
}
