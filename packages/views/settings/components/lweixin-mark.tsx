// LWeixin brand mark: an original WeChat-style double chat-bubble glyph
// drawn for this repo (no Simple Icons entry for the gateway product), so it
// keeps the "every channel has its own mark" rule without borrowing a
// trademarked logo path wholesale.
export function LweixinMark({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={className}
      fill="currentColor"
    >
      <path d="M9.5 3C5.36 3 2 5.86 2 9.39c0 1.99 1.06 3.77 2.72 4.94l-.68 2.05a.4.4 0 0 0 .58.47l2.4-1.42c.78.22 1.6.35 2.48.35.2 0 .4-.01.6-.02A5.7 5.7 0 0 1 10 14.2c0-3.14 2.98-5.7 6.65-5.7.22 0 .44.01.66.03C16.6 5.66 13.4 3 9.5 3Zm-2.6 4.1a.9.9 0 1 1 0 1.8.9.9 0 0 1 0-1.8Zm5.2 0a.9.9 0 1 1 0 1.8.9.9 0 0 1 0-1.8Z" />
      <path d="M16.65 9.5c-3.11 0-5.65 2.1-5.65 4.7 0 2.6 2.54 4.7 5.65 4.7.63 0 1.24-.09 1.8-.26l1.94 1.15a.33.33 0 0 0 .48-.39l-.55-1.66A4.36 4.36 0 0 0 22.3 14.2c0-2.6-2.54-4.7-5.65-4.7Zm-2.05 3.3a.75.75 0 1 1 0 1.5.75.75 0 0 1 0-1.5Zm4.1 0a.75.75 0 1 1 0 1.5.75.75 0 0 1 0-1.5Z" />
    </svg>
  );
}
