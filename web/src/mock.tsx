import { FormEvent, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { ApiError, Purchase, Ticket, api } from "./api";
import { Banner } from "./ui";

export function MockConfig() {
  const [params] = useSearchParams();
  const ticket = params.get("ticket") ?? "";
  const [view, setView] = useState<Ticket | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState("");
  useEffect(() => {
    if (!ticket) { setError("缺少 ticket"); return; }
    api.ticket(ticket).then(setView).catch((err) => setError(err instanceof ApiError ? err.message : "没有打开配置页"));
  }, [ticket]);
  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    try {
      await api.mockComplete(ticket, String(form.get("price_yuan") ?? ""), String(form.get("logon_id") ?? ""));
      setDone("授权完成。回到平台刷新席位，状态应为已绑定。");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "没有完成授权");
    } finally { setBusy(false); }
  }
  return (
    <div className="mock">
      <article>
        <p className="hint">模拟 · 支付宝托管配置。正式环境这一页在支付宝 App 里。</p>
        <h1>{view?.product_name || "开通个人收款"}</h1>
        <Banner>{error}</Banner>
        <Banner tone="ok">{done}</Banner>
        {view?.paid ? <p>这个席位已经配置过，登录号 {view.logon_masked || "已绑定"}。</p> : (
          <form onSubmit={(e) => void onSubmit(e)}>
            <p className="hint">授权后，顾客的付款进入这个支付宝。价格由你在这里确认，平台初始化时不传价格。</p>
            <div className="field">
              <label htmlFor="logon">支付宝登录号</label>
              <input id="logon" name="logon_id" autoComplete="username" placeholder="可留空，将生成模拟账号" />
            </div>
            <div className="field" style={{ marginTop: "0.8rem" }}>
              <label htmlFor="price">顾客永久买断价（元）</label>
              <input id="price" name="price_yuan" inputMode="decimal" placeholder="留空则用项目预设价" />
            </div>
            <div className="actions"><button className="primary" disabled={busy}>{busy ? "正在授权…" : "授权并完成配置"}</button></div>
          </form>
        )}
      </article>
    </div>
  );
}

export function MockPurchase() {
  const [params] = useSearchParams();
  const ticket = params.get("ticket") ?? "";
  const [view, setView] = useState<Ticket | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [paid, setPaid] = useState<Purchase | null>(null);
  useEffect(() => {
    if (!ticket) { setError("缺少 ticket"); return; }
    api.ticket(ticket).then((v) => { setView(v); if (v.paid) setPaid({} as Purchase); }).catch((err) => setError(err instanceof ApiError ? err.message : "没有打开购买页"));
  }, [ticket]);
  async function pay() {
    setBusy(true);
    setError("");
    try {
      setPaid(await api.mockPay(ticket));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "没有完成支付");
    } finally { setBusy(false); }
  }
  const amount = view?.price ? `¥${view.price}` : "";
  return (
    <div className="mock">
      <article>
        <p className="hint">模拟 · 支付宝托管购买。钱进入创作者绑定的支付宝，不进平台账户。</p>
        <h1>{view?.product_name || "确认购买"}</h1>
        <p className="amount num">{amount || "—"}</p>
        <p className="hint">{view?.display_name || "永久买断"}{view?.external_buyer_id ? ` · ${view.external_buyer_id}` : ""}</p>
        <Banner>{error}</Banner>
        {view?.expired && !view.paid ? <p>这个购买链接已经过期。</p> : null}
        {paid ? <p>支付完成。回到平台后，访问决策应为允许，抽成记在应收。</p> : (
          <button className="primary" type="button" disabled={busy || !view || view.expired} onClick={() => void pay()}>
            {busy ? "正在支付…" : `确认支付 ${amount}`}
          </button>
        )}
      </article>
    </div>
  );
}
