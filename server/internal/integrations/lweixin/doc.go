// Package lweixin is the Multica channel adapter for a self-hosted LWEIXIN
// bot server (Windows WeChat protocol implementation).
//
// LWEIXIN exposes a small HTTP API on a logged-in Windows Weixin session:
//
//	GET  /api/message/list?since=<seq> inbound long-poll; seq is opaque and
//	                                    server-managed
//	POST /api/message/send             {"to","content"} outbound text
//	GET  /api/status                   login/session probe
//
// The adapter mirrors the telegram adapter's shape: one blocking receive
// loop during Connect (poll /message/list), engine resolvers on the generic
// channel_* tables, verdict replies through OutboundReplier, and a bus-driven
// outbound post on EventChatDone. The device identity (Windows session,
// single online slot per account) is owned by the LWEIXIN server; this
// adapter never touches login flows or QR codes.
//
// Maintenance: COMMUNITY-MAINTAINED. See
// apps/docs/.../community-maintained.mdx for the support boundary.

package lweixin
