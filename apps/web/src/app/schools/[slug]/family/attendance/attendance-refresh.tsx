"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

export default function AttendanceRefresh() {
  const { refresh } = useRouter();
  useEffect(() => {
    const onPageShow = (event: PageTransitionEvent) => {
      if (event.persisted) refresh();
    };
    window.addEventListener("focus", refresh);
    window.addEventListener("pageshow", onPageShow);
    return () => {
      window.removeEventListener("focus", refresh);
      window.removeEventListener("pageshow", onPageShow);
    };
  }, [refresh]);
  return null;
}
