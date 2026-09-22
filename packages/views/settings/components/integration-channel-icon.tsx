import { DingTalkMark } from "./dingtalk-mark";
import { LarkMark } from "./lark-mark";
import { SlackMark } from "./slack-mark";
import { TelegramMark } from "./telegram-mark";
import { LweixinMark } from "./lweixin-mark";
import { WecomMark } from "./wecom-mark";

type IntegrationChannel = "lark" | "slack" | "dingtalk" | "wecom" | "lweixin" | "telegram";

// Every channel gets its own brand mark, never a generic lucide glyph: the icon
// is what tells a reader which platform the section belongs to, and a stand-in
// speech bubble or plug says nothing (see WecomMark, #6585). lucide-react ships
// no brand icons, so a new channel needs its own `*-mark.tsx` before it can be
// listed here.
export function IntegrationChannelIcon({ channel }: { channel: IntegrationChannel }) {
  const icon = {
    lark: <LarkMark className="h-4 w-4" />,
    slack: <SlackMark className="h-4 w-4" />,
    dingtalk: <DingTalkMark className="h-5 w-5" />,
    wecom: <WecomMark className="h-4 w-4" />,
    lweixin: <LweixinMark className="h-4 w-4" />,
    telegram: <TelegramMark className="h-4 w-4" />,
  }[channel];

  return (
    <span
      aria-hidden="true"
      data-testid={`integration-channel-icon-${channel}`}
      className="flex size-5 shrink-0 items-center justify-center text-muted-foreground"
    >
      {icon}
    </span>
  );
}
