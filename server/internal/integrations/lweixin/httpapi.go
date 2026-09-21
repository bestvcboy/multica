package lweixin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrAPIToken is a 401 from the LWEIXIN server when a configured api_token
// was rejected.
var ErrAPIToken = errAPIToken{}

type errAPIToken struct{}

func (errAPIToken) Error() string { return "lweixin: server rejected the api token" }

// statusError is any >=400 response. RetryAfter carries Retry-After on 429;
// Body keeps the detail for logs.
type statusError struct {
	Code       int
	RetryAfter time.Duration
	Body       string
}

func (e *statusError) Error() string {
	return "lweixin: http "+strconv.Itoa(e.Code)+": "+e.Body
}

func retryAfterHeader(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	secs, err := strconv.ParseFloat(v, 64)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}

// lweixinAPI is the raw HTTP client for the LWEIXIN FastAPI surface: status
// probe, message pull, text send. The {"ok":..,"data":..} envelope is
// unwrapped here so callers decode payloads directly.
type lweixinAPI struct {
	base   string
	token  string
	client *http.Client
}

func newAPI(base, token string, client *http.Client) *lweixinAPI {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &lweixinAPI{base: base, token: token, client: client}
}

func (a *lweixinAPI) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal lweixin request: %w", err)
		}
		rdr = bytesReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.url(path, query), rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read lweixin response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrAPIToken
	}
	if resp.StatusCode >= 400 {
		return &statusError{
		Code:       resp.StatusCode,
		RetryAfter: retryAfterHeader(resp.Header),
		Body:       strings.ToValidUTF8(string(data), "?"),
		}
	}
	if out == nil {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err == nil && len(env.Data) > 0 {
		data = env.Data
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode lweixin response: %w", err)
	}
	return nil
}

func (a *lweixinAPI) url(path string, q url.Values) string {
	full := a.base + path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	return full
}

// botStatus mirrors GET /api/status data.
type botStatus struct {
	State    string `json:"state"`
	Wxid     string `json:"wxid,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}

func (a *lweixinAPI) status(ctx context.Context) (botStatus, error) {
	var st botStatus
	err := a.do(ctx, http.MethodGet, "/api/status", nil, nil, &st)
	return st, err
}

// Probe is the exported one-shot status check the install handler uses to
// verify reachability and learn the bot wxid before persisting anything.
type Probe struct{ api *lweixinAPI }

// NewProbe builds a throwaway client for /api/status.
func NewProbe(baseURL, apiToken string) *Probe { return &Probe{api: newAPI(baseURL, apiToken, nil)} }

// Status fetches GET /api/status.
func (p *Probe) Status(ctx context.Context) (botStatus, error) { return p.api.status(ctx) }

// inboundMessage mirrors one row of GET /api/messages data.messages
// (wxsec/storage.py parse_message plus the seq column from received()).
type inboundMessage struct {
	Seq         int64  `json:"seq"`
	MsgID       string `json:"msg_id"`
	NewMsgID    string `json:"new_msg_id"`
	Type        int    `json:"type"`
	Content     string `json:"content"`
	CreateTime  int64  `json:"create_time"`
	IsGroup     bool   `json:"is_group"`
	GroupSender string `json:"group_sender,omitempty"`
	From        string `json:"from"`
	To          string `json:"to"`
	File        *msgAttachment `json:"file,omitempty"`
}

// msgAttachment is the type-49 file attachment block.
type msgAttachment struct {
	MediaID  string `json:"media_id"`
	FileName string `json:"file_name"`
	TotalLen int64  `json:"total_len"`
	FileExt  string `json:"file_ext"`
}

type messagesPage struct {
	Messages []inboundMessage `json:"messages"`
	LastSeq  int64            `json:"last_seq"`
	Since    int64            `json:"since"`
}

func (a *lweixinAPI) messages(ctx context.Context, since int64, limit int) (messagesPage, error) {
	q := url.Values{}
	q.Set("since", strconv.FormatInt(since, 10))
	if limit > 0 && limit != 100 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var pg messagesPage
	err := a.do(ctx, http.MethodGet, "/api/messages", q, nil, &pg)
	return pg, err
}

// sendTextResult carries the data of POST /api/message/send.
type sendTextResult struct {
	Ret   int    `json:"ret"`
	SvrID string `json:"svrid"`
}

// sendText posts a text message. ret!=0 is still reported as an error even
// though the server may have already pushed something into the session.
func (a *lweixinAPI) sendText(ctx context.Context, to, content string) (sendTextResult, error) {
	body := struct {
		To      string `json:"to"`
		Content string `json:"content"`
	}{To: to, Content: content}
	res, err := a.rawSend(ctx, body)
	if err != nil {
		return sendTextResult{}, err
	}
	if res.Ret != 0 {
		return res, fmt.Errorf("lweixin: send ret=%d to=%s", res.Ret, to)
	}
	return res, nil
}

func (a *lweixinAPI) rawSend(ctx context.Context, body any) (sendTextResult, error) {
	var res sendTextResult
	err := a.do(ctx, http.MethodPost, "/api/message/send", nil, body, &res)
	return res, err
}

// bytesReader avoids importing bytes for one adapter helper.
func bytesReader(b []byte) io.Reader { return strings.NewReader(string(b)) }
