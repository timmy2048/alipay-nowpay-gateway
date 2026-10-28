export class ApiError extends Error {
  code: string;
  constructor(code: string, message: string) {
    super(message);
    this.code = code;
  }
}

export type Actor = {
  actor_type: string;
  id: string;
  email: string;
  name: string;
  creator_id: string;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const res = await fetch(path, { ...init, headers, credentials: "include" });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    throw new ApiError(data?.error?.code ?? "error", data?.error?.message ?? "请求没有完成");
  }
  return data as T;
}

export const api = {
  health: () => req<{ status: string; alipay_mode: string; commission_engine?: string }>("/api/health"),
  me: () => req<Actor>("/api/auth/me"),
  login: (email: string, password: string, portal: string) =>
    req<Actor>("/api/auth/login", { method: "POST", body: JSON.stringify({ email, password, portal }) }),
  logout: () => req("/api/auth/logout", { method: "POST" }),
  dashboard: () => req<Dashboard>("/api/admin/dashboard"),
  creators: () => req<{ creators: Creator[] }>("/api/admin/creators"),
  createCreator: (body: Record<string, string>) =>
    req<Creator>("/api/admin/creators", { method: "POST", body: JSON.stringify(body) }),
  projects: (creatorId = "") =>
    req<{ projects: Project[] }>(`/api/admin/projects${creatorId ? `?creator_id=${creatorId}` : ""}`),
  createProject: (body: unknown) => req<Project>("/api/admin/projects", { method: "POST", body: JSON.stringify(body) }),
  updateProject: (id: string, body: unknown) =>
    req<Project>(`/api/admin/projects/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  desk: (id: string) => req<Desk>(`/api/admin/projects/${id}`),
  openSeat: (projectId: string, payNow: boolean) =>
    req<Seat>(`/api/admin/projects/${projectId}/seats`, {
      method: "POST",
      body: JSON.stringify({ pay_now: payNow, channel: payNow ? "admin_recorded" : "" }),
    }),
  paySeat: (id: string) => req<Seat>(`/api/admin/seats/${id}/pay`, { method: "POST", body: "{}" }),
  bind: (id: string) => req<Seat>(`/api/admin/seats/${id}/bind`, { method: "POST", body: "{}" }),
  completeBind: (id: string, priceYuan: string, logonId: string) =>
    req<Seat>(`/api/admin/seats/${id}/bind/complete`, {
      method: "POST",
      body: JSON.stringify({ price_yuan: priceYuan, logon_id: logonId }),
    }),
  simulate: (id: string, buyer: string) =>
    req<Purchase>(`/api/admin/seats/${id}/purchases`, {
      method: "POST",
      body: JSON.stringify({ external_buyer_id: buyer }),
    }),
  pricing: () => req<{ catalog: PriceItem[]; phase1_mode: string }>("/api/admin/pricing"),
  savePricing: (name: string, amountYuan: string) =>
    req("/api/admin/pricing", { method: "PUT", body: JSON.stringify({ name, amount_yuan: amountYuan }) }),
  commission: () => req<Commission>("/api/admin/commission"),
  saveCommission: (body: Record<string, string>) =>
    req<Commission>("/api/admin/commission", { method: "PUT", body: JSON.stringify(body) }),
  ledger: () => req<{ entries: LedgerEntry[] }>("/api/admin/ledger?limit=80"),
  settle: (note: string) =>
    req<Settlement>("/api/admin/settlements", { method: "POST", body: JSON.stringify({ note }) }),
  audit: () => req<{ entries: Audit[] }>("/api/admin/audit?limit=80"),
  webhooks: () => req<{ deliveries: Webhook[] }>("/api/admin/webhooks?limit=40"),
  password: (current: string, next: string) =>
    req("/api/admin/password", {
      method: "PUT",
      body: JSON.stringify({ current_password: current, new_password: next }),
    }),
  creatorProjects: () => req<{ projects: Project[] }>("/api/creator/projects"),
  creatorDesk: (id: string) => req<Desk>(`/api/creator/projects/${id}`),
  saveEndpoints: (id: string, callback: string, notify: string) =>
    req<Project>(`/api/creator/projects/${id}/endpoints`, {
      method: "PUT",
      body: JSON.stringify({ callback_url: callback, notify_url: notify }),
    }),
  checkout: (projectId: string) =>
    req<Seat>(`/api/creator/projects/${projectId}/seats`, { method: "POST", body: "{}" }),
  creatorBind: (id: string) => req<Seat>(`/api/creator/seats/${id}/bind`, { method: "POST", body: "{}" }),
  creatorComplete: (id: string, priceYuan: string, logonId: string) =>
    req<Seat>(`/api/creator/seats/${id}/bind/complete`, {
      method: "POST",
      body: JSON.stringify({ price_yuan: priceYuan, logon_id: logonId }),
    }),
  ticket: (ticket: string) => req<Ticket>(`/api/mock/alipay/tickets/${ticket}`),
  mockComplete: (ticket: string, priceYuan: string, logonId: string) =>
    req<Seat>(`/api/mock/alipay/tickets/${ticket}/complete`, {
      method: "POST",
      body: JSON.stringify({ price_yuan: priceYuan, logon_id: logonId }),
    }),
  mockPay: (ticket: string) => req<Purchase>(`/api/mock/alipay/tickets/${ticket}/pay`, { method: "POST", body: "{}" }),
};

export type Creator = {
  id: string;
  public_code: string;
  name: string;
  contact_email: string;
  phone: string;
  status: string;
  notes: string;
  portal_email?: string;
  created_at: string;
};

export type Project = {
  id: string;
  public_code: string;
  creator_id: string;
  creator_name: string;
  name: string;
  status: string;
  callback_url: string;
  notify_url: string;
  billing_mode: string;
  seat_price_cents: number | null;
  customer_price_cents: number;
  commission_bps: number | null;
  effective_seat_price_cents: number;
  effective_commission_bps: number;
  seat_count: number;
  bound_count: number;
};

export type Option = { sku_id: string; display_name: string; price: string; billing_mode: string };

export type Seat = {
  id: string;
  public_code: string;
  ordinal: number;
  status: string;
  amount_cents: number;
  capability_status: string;
  alipay_user_id: string;
  alipay_logon_masked: string;
  configuration_url: string;
  configuration_qr_code: string;
  management_url: string;
  billing_mode_actual: string;
  charging_options: Option[];
  project_id: string;
  project_name?: string;
  creator_name?: string;
};

export type Purchase = {
  id: string;
  external_buyer_id: string;
  gross_cents: number;
  status: string;
  decision: string;
  alipay_trade_no: string;
  commission_cents: number;
  commission_bps: number;
  alipay_fee_cents: number;
  creator_net_cents: number;
  estimates_exceed_gross: boolean;
  purchase_qr_code: string;
  paid_at: string | null;
  created_at: string;
};

export type Desk = { project: Project; seats: Seat[]; purchases: Purchase[] };

export type PriceItem = {
  id: string;
  name: string;
  billing_mode: string;
  quota_unit?: string;
  duration_period?: string;
  amount_cents: number;
  purchasable: boolean;
  active: boolean;
};

export type Commission = {
  rate_bps: number;
  alipay_jsapi_fee_bps: number;
  alipay_nowpay_fee_bps: number;
};

export type LedgerEntry = {
  id: string;
  account: string;
  direction: string;
  amount_cents: number;
  status: string;
  memo: string;
  created_at: string;
};

export type Settlement = { id: string; total_cents: number; entry_count: number; note: string };

export type Audit = {
  id: number;
  actor_type: string;
  action: string;
  resource_type: string;
  resource_id: string;
  created_at: string;
};

export type Webhook = {
  id: string;
  event_type: string;
  target_url: string;
  status: string;
  last_error: string;
  created_at: string;
};

export type Need = {
  seat_id: string;
  ordinal: number;
  status: string;
  amount_cents: number;
  project_id: string;
  project_name: string;
  creator_name: string;
};

export type Dashboard = {
  creator_count: number;
  project_count: number;
  bound_seats: number;
  unpaid_seats: number;
  configuring_seats: number;
  paid_purchase_count: number;
  gross_cents: number;
  commission_booked_cents: number;
  commission_settled_cents: number;
  needs: Need[];
  recent: Purchase[];
};

export type Ticket = {
  kind: string;
  product_name: string;
  price: string;
  currency: string;
  display_name: string;
  paid: boolean;
  expired: boolean;
  capability_status: string;
  logon_masked: string;
  external_buyer_id: string;
};
