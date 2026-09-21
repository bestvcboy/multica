package lweixin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
)

// TypeLweixin is the channel discriminator for the LWEIXIN adapter. Defined
// here (not in the channel core) so registering the platform never edits the
// core, mirroring TypeTelegram / TypeFeishu.
const TypeLweixin channel.Type = "lweixin"

// installConfig is the JSON shape stored in channel_installation.config.
// base_url points at the LWEIXIN HTTP root (e.g. http://10.0.0.8:8000);
// app_id holds the bot account wxid and fills the generic
// (channel_type, config->>'app_id') routing slot - the polling loop serves
// exactly one installation, but the unique index still pins one account to
// one agent across all workspaces.
type installConfig struct {
	AppID             string `json:"app_id"`
	BaseURL           string `json:"base_url"`
	APITokenEncrypted string `json:"api_token_encrypted,omitempty"`
}

// credentials is the decrypted runtime form the HTTP client dials with.
type credentials struct {
	AppID    string
	BaseURL  string
	APIToken string
}

// Decrypter turns stored ciphertext into plaintext. The wiring injects a
// secretbox-backed implementation; nil treats stored bytes as plaintext
// (tests).
type Decrypter func(ciphertext []byte) (plaintext []byte, err error)

// decodeCredentials is the single place the LWEIXIN config JSON is
// interpreted and the optional token decrypted.
func decodeCredentials(raw json.RawMessage, decrypt Decrypter) (credentials, error) {
	if len(raw) == 0 {
		return credentials{}, errors.New("lweixin: empty installation config")
	}
	var cfg installConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return credentials{}, fmt.Errorf("decode lweixin installation config: %w", err)
	}
	token, err := decryptToken(cfg.APITokenEncrypted, decrypt)
	if err != nil {
		return credentials{}, fmt.Errorf("decrypt lweixin api token: %w", err)
	}
	if cfg.BaseURL == "" {
		return credentials{}, errors.New("lweixin: installation has no base_url")
	}
	return credentials{AppID: cfg.AppID, BaseURL: cfg.BaseURL, APIToken: token}, nil
}

// PublicConfig is the non-secret subset surfaced on the management API.
type PublicConfig struct {
	AppID   string `json:"app_id"`
	BaseURL string `json:"base_url"`
}

// DecodePublicConfig extracts display-safe fields; a decode miss yields the
// zero value so the management list still renders the row.
func DecodePublicConfig(raw json.RawMessage) PublicConfig {
	var cfg installConfig
	_ = json.Unmarshal(raw, &cfg)
	return PublicConfig{AppID: cfg.AppID, BaseURL: cfg.BaseURL}
}

// decryptToken base64-decodes the stored ciphertext and runs it through the
// injected decrypter; empty ciphertext yields empty token.
func decryptToken(enc string, decrypt Decrypter) (string, error) {
	if enc == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(stripSpace(enc))
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	if decrypt == nil {
		return string(data), nil
	}
	plain, err := decrypt(data)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
}

