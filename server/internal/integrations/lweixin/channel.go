package lweixin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
)

// lweixinChannel is one installation's inbound polling loop, mirroring
// telegramChannel: the LWEIXIN server exposes a pull API guarded by its own
// seq cursor, so the persistent-connection equivalent is a short poll. One
// loop per installation, supervised by engine.Supervisor like every other
// channel adapter.
type lweixinChannel struct {
	api     *lweixinAPI
	handler channel.InboundHandler
	logger  *slog.Logger

	// selfWxid is the bot account wxid from the installation config; used to
	// translate the pull API's from/to orientation into a source-centric view.
	selfWxid string
}

// pollRetryDelay spaces retries after a transient pull failure inside one
// Connect attempt before giving the error to the Supervisor's backoff.
const pollRetryDelay = 2 * time.Second

// pollInterval spaces idle polls against /api/messages. The upstream server
// returns immediately with whatever it has, so this is a plain cadence.
const pollInterval = 2 * time.Second

// pollLimit is the page size per pull; the LWEIXIN API caps it at 500.
const pollLimit = 200

// sleepCtx sleeps for d, returning early once ctx is done. It reports
// whether the sleep completed without cancellation.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select{
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// sleepCtx sleeps for d, returning early once ctx is done. It reports
func (c *lweixinChannel) Type() channel.Type { return TypeLweixin }

// Capabilities: text only. The LWEIXIN server accepts media sends in later
// work; inbound media does not round-trip v1 either.
func (c *lweixinChannel) Capabilities() channel.Capability { return channel.CapText }

// Disconnect is a no-op: the polling loop's lifetime is scoped to Connect.
func (c *lweixinChannel) Disconnect(ctx context.Context) error { return nil }

// Send delivers one outbound text via the installation's LWEIXIN server.
func (c *lweixinChannel) Send(ctx context.Context, out channel.OutboundMessage) (channel.SendResult, error) {
	_, err := c.api.sendText(ctx, out.ChatID, out.Text)
	if err != nil {
		return channel.SendResult{}, err
	}
	return channel.SendResult{}, nil
}

// Connect runs the /api/messages polling loop until ctx is cancelled (nil) or
// a handler error propagates (the Supervisor reconnects under backoff). The
// offset starts at 0: the first poll re-delivers the tail of history and the
// engine's (installation, message_id) dedup absorbs replays — the same
// tolerance the Telegram getUpdates loop relies on.
func (c *lweixinChannel) Connect(ctx context.Context) error {
	if c.handler == nil {
		return errors.New("lweixin: inbound handler not configured")
	}
	var offset int64
	for {
		page, err := c.api.messages(ctx, offset, pollLimit)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var se *statusError
			if errors.As(err, &se) && se.Code == 429 && se.RetryAfter > 0 {
				c.logger.WarnContext(ctx, "lweixin: messages rate limited", "retry_after", se.RetryAfter)
				if !sleepCtx(ctx, se.RetryAfter) {
					return nil
				}
				continue
			}
			c.logger.WarnContext(ctx, "lweixin: messages poll failed", "error", err)
			if !sleepCtx(ctx, pollRetryDelay) {
				return nil
			}
			return fmt.Errorf("lweixin: messages poll: %w", err)
		}
		for _, m := range page.Messages {
			if m.Seq >= offset {
				offset = m.Seq + 1
			}
			msg, ok := inboundFromMessage(m, c.selfWxid)
			if !ok {
				continue
			}
			if err := c.handler(ctx, msg); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
		if !sleepCtx(ctx, pollInterval) {
			return nil
		}
	}
}

// ChannelDeps are the shared dependencies the Factory closes over. The engine
// inbound handler is supplied per-build via channel.Config.Handler.
type ChannelDeps struct {
	Decrypt Decrypter
	Logger  *slog.Logger
	// HTTPClient overrides the polling client (tests). Nil uses a default.
	HTTPClient *http.Client
}

// RegisterLweixin registers the per-installation Factory so the engine
// Supervisor builds + supervises one polling loop per active LWEIXIN
// installation. Same contract as telegram.RegisterTelegram — no engine edit.
func RegisterLweixin(reg *channel.Registry, deps ChannelDeps) {
	reg.Register(TypeLweixin, newLweixinFactory(deps))
}

func newLweixinFactory(deps ChannelDeps) channel.Factory {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(cfg channel.Config) (channel.Channel, error) {
		creds, err := decodeCredentials(cfg.Raw, deps.Decrypt)
		if err != nil {
			return nil, fmt.Errorf("lweixin: load credentials: %w", err)
		}
		return &lweixinChannel{
			api:     newAPI(creds.BaseURL, creds.APIToken, deps.HTTPClient),
			handler: cfg.Handler,
			logger:  logger,
			selfWxid: creds.AppID,
		}, nil
	}
}
