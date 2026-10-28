import { useEffect, useRef } from "react";
import QRCode from "qrcode";

export function QR({ value }: { value: string }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    if (!ref.current || !value) return;
    void QRCode.toCanvas(ref.current, value, { width: 168, margin: 1, color: { dark: "#1c1712", light: "#fffdf8" } });
  }, [value]);
  return <canvas ref={ref} aria-label="二维码" />;
}

export function Banner({ tone, children }: { tone?: "ok" | "bad"; children: string }) {
  if (!children) return null;
  return (
    <p className="banner" data-tone={tone === "ok" ? "ok" : undefined} role={tone === "ok" ? "status" : "alert"}>
      {children}
    </p>
  );
}

export function Dot({ kind, children }: { kind: string; children: string }) {
  return (
    <span className="status" data-kind={kind}>
      <i aria-hidden="true" />
      {children}
    </span>
  );
}
