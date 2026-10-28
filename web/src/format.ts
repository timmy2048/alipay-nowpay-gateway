export function yuan(cents: number | null | undefined): string {
  if (cents == null || Number.isNaN(cents)) return "—";
  const neg = cents < 0;
  const v = Math.abs(Math.trunc(cents));
  return `${neg ? "-" : ""}${Math.floor(v / 100)}.${String(v % 100).padStart(2, "0")}`;
}

export function percentFromBps(bps: number | null | undefined): string {
  if (bps == null) return "";
  return `${Math.floor(bps / 100)}.${String(bps % 100).padStart(2, "0")}`;
}

export function when(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(d);
}

const seatText: Record<string, string> = {
  pending_payment: "待买断",
  paid_unbound: "已买断，未绑定",
  configuring: "等待授权",
  bound: "已绑定",
  disabled: "已停用",
};

const capText: Record<string, string> = {
  NOT_CONFIGURED: "未配置",
  CONFIGURING: "配置中",
  PENDING: "审核中",
  ACTION_REQUIRED: "待处理",
  ENABLED: "可收款",
  DISABLED: "已停收",
  TERMINATED: "已归档",
};

export function seatLabel(status: string) {
  return seatText[status] ?? status;
}

export function capLabel(status: string) {
  return capText[status] ?? status;
}
