"use client";

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { LweixinBindPage } from "@multica/views/lweixin";

// /lweixin/bind?token=<raw> is the gateway's "link your account" destination.
// Suspense wraps useSearchParams per Next.js 15's CSR-bailout rule.
function LweixinBindPageContent() {
  const searchParams = useSearchParams();
  const token = searchParams.get("token");
  return <LweixinBindPage token={token} />;
}

export default function Page() {
  return (
    <Suspense fallback={null}>
      <LweixinBindPageContent />
    </Suspense>
  );
}
