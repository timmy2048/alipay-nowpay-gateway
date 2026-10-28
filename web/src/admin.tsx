import { FormEvent, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ApiError, Commission, Creator, Dashboard, PriceItem, Project, api } from "./api";
import { capLabel, percentFromBps, seatLabel, when, yuan } from "./format";
import { Banner, Dot, QR } from "./ui";

function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let live = true;
    setError("");
    load()
      .then((v) => { if (live) setData(v); })
      .catch((err) => { if (live) setError(err instanceof ApiError ? err.message : "没有读到数据"); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick]);
  return { data, error, setError, reload: () => setTick((n) => n + 1), setData };
}

export function DashboardPage() {
  const { data, error } = useLoad(() => api.dashboard(), []);
  return (
    <>
      <header className="mast">
        <h1>今日账</h1>
        <hr className="rule" />
        {data ? (
          <p className="lede">
            未结算抽成 <span className="num">¥{yuan(data.commission_booked_cents)}</span>
            ，已结算 <span className="num">¥{yuan(data.commission_settled_cents)}</span>
            。已绑定席位 {data.bound_seats} 个，顾客已付 <span className="num">¥{yuan(data.gross_cents)}</span>
            （{data.paid_purchase_count} 笔）。
          </p>
        ) : <p className="lede">正在汇总席位和抽成。</p>}
      </header>
      <Banner>{error}</Banner>
      <section className="section">
        <h2>还要处理</h2>
        {!data || data.needs.length === 0 ? <p className="empty">没有待买断或待绑定的席位。去项目里为创作者开一个收款席位。</p> : (
          <table>
            <thead><tr><th>项目</th><th>创作者</th><th>席位</th><th>状态</th><th>买断价</th></tr></thead>
            <tbody>
              {data.needs.map((n) => (
                <tr key={n.seat_id}>
                  <td><Link to={`/admin/projects/${n.project_id}`}>{n.project_name}</Link></td>
                  <td>{n.creator_name}</td>
                  <td>第 {n.ordinal} 席</td>
                  <td><Dot kind={n.status}>{seatLabel(n.status)}</Dot></td>
                  <td className="num">¥{yuan(n.amount_cents)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      <Recent data={data} />
    </>
  );
}

function Recent({ data }: { data: Dashboard | null }) {
  return (
    <section className="section">
      <h2>最近的顾客付款</h2>
      {!data || data.recent.length === 0 ? <p className="empty">还没有入账。绑定席位之后，可以在项目里模拟一笔顾客付款。</p> : (
        <table>
          <thead><tr><th>时间</th><th>买家</th><th>实付</th><th>抽成</th><th>决策</th></tr></thead>
          <tbody>
            {data.recent.map((p) => (
              <tr key={p.id}>
                <td>{when(p.paid_at ?? p.created_at)}</td>
                <td>{p.external_buyer_id}</td>
                <td className="num">¥{yuan(p.gross_cents)}</td>
                <td className="num">¥{yuan(p.commission_cents)}</td>
                <td>{p.decision || p.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

export function CreatorsPage() {
  const { data, error, setError, reload } = useLoad(() => api.creators(), []);
  const [busy, setBusy] = useState(false);
  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    try {
      await api.createCreator({
        name: String(form.get("name") ?? ""),
        contact_email: String(form.get("contact_email") ?? ""),
        phone: String(form.get("phone") ?? ""),
        notes: String(form.get("notes") ?? ""),
        portal_email: String(form.get("portal_email") ?? ""),
        portal_name: String(form.get("portal_name") ?? ""),
        portal_password: String(form.get("portal_password") ?? ""),
      });
      e.currentTarget.reset();
      reload();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "没有建成");
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <header className="mast">
        <h1>创作者</h1>
        <hr className="rule" />
        <p className="lede">建档之后才能开项目。门户密码给他们登录，自己买断席位、扫码绑定。</p>
      </header>
      <Banner>{error}</Banner>
      <form onSubmit={onSubmit}>
        <div className="grid-form">
          <div className="field"><label htmlFor="cname">名称</label><input id="cname" name="name" required autoComplete="name" /></div>
          <div className="field"><label htmlFor="cphone">电话</label><input id="cphone" name="phone" autoComplete="tel" /></div>
          <div className="field"><label htmlFor="cmail">联系邮箱</label><input id="cmail" name="contact_email" type="email" autoComplete="email" /></div>
          <div className="field"><label htmlFor="pmail">门户登录邮箱</label><input id="pmail" name="portal_email" type="email" required autoComplete="off" /></div>
          <div className="field"><label htmlFor="pname">门户显示名</label><input id="pname" name="portal_name" autoComplete="name" /></div>
          <div className="field"><label htmlFor="ppass">门户密码</label><input id="ppass" name="portal_password" type="text" required minLength={8} autoComplete="new-password" /></div>
          <div className="field wide"><label htmlFor="notes">备注</label><textarea id="notes" name="notes" /></div>
        </div>
        <div className="actions"><button className="primary" disabled={busy}>{busy ? "正在建档…" : "建立创作者"}</button></div>
      </form>
      {!data || data.creators.length === 0 ? <p className="empty">还没有创作者。</p> : (
        <table>
          <thead><tr><th>名称</th><th>门户</th><th>状态</th><th>编号</th></tr></thead>
          <tbody>
            {data.creators.map((c: Creator) => (
              <tr key={c.id}>
                <td>{c.name}</td>
                <td>{c.portal_email || "—"}</td>
                <td>{c.status === "active" ? "在营" : "停用"}</td>
                <td className="num">{c.public_code}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

export function ProjectsPage() {
  const loaded = useLoad(() => Promise.all([api.projects(), api.creators()]), []);
  const [busy, setBusy] = useState(false);
  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    loaded.setError("");
    try {
      await api.createProject({
        creator_id: String(form.get("creator_id")),
        name: String(form.get("name")),
        customer_price_yuan: String(form.get("customer_price_yuan")),
        seat_price_yuan: String(form.get("seat_price_yuan") ?? ""),
        commission_percent: String(form.get("commission_percent") ?? ""),
        callback_url: String(form.get("callback_url") ?? ""),
        notify_url: String(form.get("notify_url") ?? ""),
        billing_mode: "ONE_TIME",
        status: "active",
      });
      e.currentTarget.reset();
      loaded.reload();
    } catch (err) {
      loaded.setError(err instanceof ApiError ? err.message : "没有建成项目");
    } finally {
      setBusy(false);
    }
  }
  const projects = loaded.data?.[0].projects ?? [];
  const creators = loaded.data?.[1].creators ?? [];
  return (
    <>
      <header className="mast">
        <h1>项目</h1>
        <hr className="rule" />
        <p className="lede">一个项目归一位创作者。每多绑一个支付宝账号，就按当时的席位价再买断一席。阶段一只做永久买断。</p>
      </header>
      <Banner>{loaded.error}</Banner>
      <form onSubmit={onSubmit}>
        <div className="grid-form">
          <div className="field">
            <label htmlFor="creator">创作者</label>
            <select id="creator" name="creator_id" required defaultValue="">
              <option value="" disabled>选择</option>
              {creators.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>
          <div className="field"><label htmlFor="pname">项目名称</label><input id="pname" name="name" required /></div>
          <div className="field"><label htmlFor="cprice">顾客买断价（元）</label><input id="cprice" name="customer_price_yuan" inputMode="decimal" required placeholder="29.90" /></div>
          <div className="field"><label htmlFor="sprice">席位买断价（元，可空）</label><input id="sprice" name="seat_price_yuan" inputMode="decimal" placeholder="沿用目录价" /></div>
          <div className="field"><label htmlFor="crate">项目抽成 %（可空）</label><input id="crate" name="commission_percent" inputMode="decimal" placeholder="沿用全局" /></div>
          <div className="field"><label htmlFor="cb">支付完成回跳</label><input id="cb" name="callback_url" placeholder="https://..." /></div>
          <div className="field wide"><label htmlFor="nf">异步通知</label><input id="nf" name="notify_url" placeholder="https://..." /></div>
        </div>
        <div className="actions"><button className="primary" disabled={busy || creators.length === 0}>{busy ? "正在创建…" : "创建项目"}</button></div>
      </form>
      {projects.length === 0 ? <p className="empty">还没有项目。</p> : (
        <table>
          <thead><tr><th>项目</th><th>创作者</th><th>席位价</th><th>抽成</th><th>席位</th></tr></thead>
          <tbody>
            {projects.map((p: Project) => (
              <tr key={p.id}>
                <td><Link to={`/admin/projects/${p.id}`}>{p.name}</Link></td>
                <td>{p.creator_name}</td>
                <td className="num">¥{yuan(p.effective_seat_price_cents)}</td>
                <td className="num">{percentFromBps(p.effective_commission_bps)}%</td>
                <td>{p.bound_count}/{p.seat_count} 已绑定</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

export function DeskPage() {
  const { id = "" } = useParams();
  const loaded = useLoad(() => api.desk(id), [id]);
  const [busy, setBusy] = useState("");
  const [note, setNote] = useState("");
  const desk = loaded.data;
  async function run(key: string, fn: () => Promise<unknown>, okText: string) {
    setBusy(key);
    loaded.setError("");
    setNote("");
    try {
      await fn();
      setNote(okText);
      loaded.reload();
    } catch (err) {
      loaded.setError(err instanceof ApiError ? err.message : "操作没有完成");
    } finally {
      setBusy("");
    }
  }
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!desk) return;
    const form = new FormData(e.currentTarget);
    await run("save", () => api.updateProject(desk.project.id, {
      creator_id: desk.project.creator_id,
      name: String(form.get("name")),
      status: String(form.get("status")),
      callback_url: String(form.get("callback_url") ?? ""),
      notify_url: String(form.get("notify_url") ?? ""),
      billing_mode: "ONE_TIME",
      customer_price_yuan: String(form.get("customer_price_yuan")),
      seat_price_yuan: String(form.get("seat_price_yuan") ?? ""),
      commission_percent: String(form.get("commission_percent") ?? ""),
    }), "项目已保存。之后新开的席位用新价格。");
  }
  return (
    <>
      <header className="mast">
        <h1>{desk?.project.name ?? "项目"}</h1>
        <hr className="rule" />
        <p className="lede">{desk ? `${desk.project.creator_name} · 当前席位价 ¥${yuan(desk.project.effective_seat_price_cents)} · 抽成 ${percentFromBps(desk.project.effective_commission_bps)}%` : "正在打开项目。"}</p>
      </header>
      <Banner>{loaded.error}</Banner>
      <Banner tone="ok">{note}</Banner>
      {desk && (
        <form key={desk.project.id} onSubmit={(e) => void save(e)}>
          <div className="grid-form">
            <div className="field"><label htmlFor="name">名称</label><input id="name" name="name" defaultValue={desk.project.name} required /></div>
            <div className="field">
              <label htmlFor="status">状态</label>
              <select id="status" name="status" defaultValue={desk.project.status}><option value="active">在营</option><option value="suspended">停用</option></select>
            </div>
            <div className="field"><label htmlFor="customer">顾客买断价（元）</label><input id="customer" name="customer_price_yuan" defaultValue={yuan(desk.project.customer_price_cents)} required /></div>
            <div className="field"><label htmlFor="seat">席位覆盖价（元，空=目录）</label><input id="seat" name="seat_price_yuan" defaultValue={desk.project.seat_price_cents == null ? "" : yuan(desk.project.seat_price_cents)} /></div>
            <div className="field"><label htmlFor="rate">抽成覆盖 %（空=全局）</label><input id="rate" name="commission_percent" defaultValue={desk.project.commission_bps == null ? "" : percentFromBps(desk.project.commission_bps)} /></div>
            <div className="field"><label htmlFor="callback">回跳地址</label><input id="callback" name="callback_url" defaultValue={desk.project.callback_url} /></div>
            <div className="field wide"><label htmlFor="notify">异步通知</label><input id="notify" name="notify_url" defaultValue={desk.project.notify_url} /></div>
          </div>
          <div className="actions">
            <button className="primary" disabled={busy !== ""}>{busy === "save" ? "正在保存…" : "保存项目"}</button>
            <button type="button" disabled={busy !== ""} onClick={() => void run("seat", () => api.openSeat(desk.project.id, true), "已按当前价格登记一席买断。")}>
              {busy === "seat" ? "正在登记…" : `新开席位并登记买断 ¥${yuan(desk.project.effective_seat_price_cents)}`}
            </button>
          </div>
        </form>
      )}
      <section className="section">
        <h2>收款席位</h2>
        {!desk || desk.seats.length === 0 ? <p className="empty">还没有席位。再绑一个支付宝账号，就再买一席。</p> : (
          desk.seats.map((seat) => (
            <article key={seat.id} className="section">
              <h2>第 {seat.ordinal} 席 · ¥{yuan(seat.amount_cents)}</h2>
              <p className="hint">
                <Dot kind={seat.status}>{seatLabel(seat.status)}</Dot>
                {" · "}
                <Dot kind={seat.capability_status}>{capLabel(seat.capability_status)}</Dot>
                {seat.alipay_logon_masked ? ` · ${seat.alipay_logon_masked}` : ""}
                {seat.charging_options?.[0] ? ` · 在售 ${seat.charging_options[0].display_name} ¥${seat.charging_options[0].price}` : ""}
              </p>
              <div className="actions">
                {seat.status === "pending_payment" && (
                  <button type="button" disabled={busy !== ""} onClick={() => void run(seat.id + "pay", () => api.paySeat(seat.id), "买断已登记。")}>登记买断</button>
                )}
                {seat.status !== "pending_payment" && (
                  <button type="button" disabled={busy !== ""} onClick={() => void run(seat.id + "bind", () => api.bind(seat.id), "已向模拟网关要到配置入口。")}>生成绑定二维码</button>
                )}
                {(seat.status === "configuring" || seat.status === "paid_unbound") && (
                  <button type="button" className="primary" disabled={busy !== ""} onClick={() => void run(seat.id + "done", () => api.completeBind(seat.id, "", ""), "模拟授权完成，席位已绑定。")}>模拟完成支付宝授权</button>
                )}
                {seat.status === "bound" && seat.capability_status === "ENABLED" && (
                  <button type="button" className="primary" disabled={busy !== ""} onClick={() => void run(seat.id + "buy", () => api.simulate(seat.id, ""), "顾客付款已入账，抽成记在应收。")}>
                    模拟顾客付款{seat.charging_options?.[0] ? ` ¥${seat.charging_options[0].price}` : ""}
                  </button>
                )}
              </div>
              {seat.configuration_qr_code && seat.status !== "bound" && (
                <div className="qr">
                  <QR value={seat.configuration_qr_code} />
                  <p className="hint">用手机打开二维码，或在这台电脑上<a href={seat.configuration_qr_code}>打开模拟支付宝配置页</a>。正式环境这里是 alipays:// 链接。</p>
                </div>
              )}
            </article>
          ))
        )}
      </section>
      <section className="section">
        <h2>顾客付款</h2>
        {!desk || desk.purchases.length === 0 ? <p className="empty">这个项目还没有顾客付款。</p> : (
          <table>
            <thead><tr><th>时间</th><th>买家</th><th>实付</th><th>支付宝费估计</th><th>平台抽成</th><th>创作者约计</th><th>决策</th></tr></thead>
            <tbody>
              {desk.purchases.map((p) => (
                <tr key={p.id}>
                  <td>{when(p.paid_at ?? p.created_at)}</td>
                  <td>{p.external_buyer_id}</td>
                  <td className="num">¥{yuan(p.gross_cents)}</td>
                  <td className="num">¥{yuan(p.alipay_fee_cents)}</td>
                  <td className="num">¥{yuan(p.commission_cents)}</td>
                  <td className="num">¥{yuan(p.creator_net_cents)}</td>
                  <td>{p.decision || p.status}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}

export function PricingPage() {
  const loaded = useLoad(() => Promise.all([api.pricing(), api.commission()]), []);
  const [busy, setBusy] = useState("");
  const catalog = loaded.data?.[0].catalog ?? [];
  const commission: Commission | undefined = loaded.data?.[1];
  const one = catalog.find((c: PriceItem) => c.billing_mode === "ONE_TIME");
  async function savePrice(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy("price");
    loaded.setError("");
    try {
      await api.savePricing(String(form.get("name")), String(form.get("amount_yuan")));
      loaded.reload();
    } catch (err) {
      loaded.setError(err instanceof ApiError ? err.message : "没有保存价格");
    } finally { setBusy(""); }
  }
  async function saveRate(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy("rate");
    loaded.setError("");
    try {
      await api.saveCommission({
        rate_percent: String(form.get("rate_percent")),
        alipay_jsapi_fee_percent: String(form.get("alipay_jsapi_fee_percent")),
        alipay_nowpay_fee_percent: String(form.get("alipay_nowpay_fee_percent")),
      });
      loaded.reload();
    } catch (err) {
      loaded.setError(err instanceof ApiError ? err.message : "没有保存抽成");
    } finally { setBusy(""); }
  }
  return (
    <>
      <header className="mast">
        <h1>席位与抽成</h1>
        <hr className="rule" />
        <p className="lede">席位价是创作者付给 YIN 的永久买断。抽成是顾客付款之后记在平台账上的比例，和支付宝自己扣的手续费分开。改价只影响之后新开的席位。</p>
      </header>
      <Banner>{loaded.error}</Banner>
      <form onSubmit={(e) => void savePrice(e)}>
        <div className="grid-form">
          <div className="field"><label htmlFor="pname">席位名称</label><input id="pname" name="name" defaultValue={one?.name ?? ""} key={one?.name} /></div>
          <div className="field"><label htmlFor="amount">永久买断价（元）</label><input id="amount" name="amount_yuan" required defaultValue={one ? yuan(one.amount_cents) : ""} key={one?.amount_cents} /></div>
        </div>
        <div className="actions"><button className="primary" disabled={busy !== ""}>{busy === "price" ? "正在保存…" : "保存席位价"}</button></div>
      </form>
      <table>
        <thead><tr><th>模式</th><th>说明</th><th>价格</th><th>阶段一</th></tr></thead>
        <tbody>
          {catalog.map((item) => (
            <tr key={item.id}>
              <td>{item.billing_mode}</td>
              <td>{item.name}{item.quota_unit ? ` · ${item.quota_unit}` : ""}{item.duration_period ? ` · ${item.duration_period}` : ""}</td>
              <td className="num">¥{yuan(item.amount_cents)}</td>
              <td>{item.purchasable ? "可售" : "已预留，暂不售卖"}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <section className="section">
        <h2>平台抽成</h2>
        <p className="hint">支付宝收单和即刻收经营费只作展示，不进我们的现金账户。平台抽成先记应收，等实际收到再在分录里结算。</p>
        {commission && (
          <form onSubmit={(e) => void saveRate(e)}>
            <div className="grid-form">
              <div className="field"><label htmlFor="rate">平台抽成 %</label><input id="rate" name="rate_percent" required defaultValue={percentFromBps(commission.rate_bps)} key={commission.rate_bps} /></div>
              <div className="field"><label htmlFor="jsapi">收单费估计 %</label><input id="jsapi" name="alipay_jsapi_fee_percent" required defaultValue={percentFromBps(commission.alipay_jsapi_fee_bps)} key={commission.alipay_jsapi_fee_bps} /></div>
              <div className="field"><label htmlFor="np">即刻收估计 %</label><input id="np" name="alipay_nowpay_fee_percent" required defaultValue={percentFromBps(commission.alipay_nowpay_fee_bps)} key={commission.alipay_nowpay_fee_bps} /></div>
            </div>
            <div className="actions"><button className="primary" disabled={busy !== ""}>{busy === "rate" ? "正在保存…" : "保存抽成"}</button></div>
          </form>
        )}
      </section>
    </>
  );
}

export function LedgerPage() {
  const loaded = useLoad(() => api.ledger(), []);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  async function settle(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    loaded.setError("");
    setNote("");
    try {
      const batch = await api.settle(String(form.get("note") ?? ""));
      setNote(`已模拟结算 ¥${yuan(batch.total_cents)}，共 ${batch.entry_count} 笔应收。`);
      loaded.reload();
    } catch (err) {
      loaded.setError(err instanceof ApiError ? err.message : "没有结算");
    } finally { setBusy(false); }
  }
  const entries = loaded.data?.entries ?? [];
  return (
    <>
      <header className="mast">
        <h1>分录</h1>
        <hr className="rule" />
        <p className="lede">顾客的本金记在备忘里，钱在创作者的支付宝。平台只把抽成先挂应收，结算时再记一笔现金。这是 mock_book_then_settle。</p>
      </header>
      <Banner>{loaded.error}</Banner>
      <Banner tone="ok">{note}</Banner>
      <form onSubmit={(e) => void settle(e)}>
        <div className="field" style={{ maxWidth: "28rem" }}>
          <label htmlFor="note">结算备注</label>
          <input id="note" name="note" placeholder="例如：10 月手工结算" />
        </div>
        <div className="actions"><button className="primary" disabled={busy}>{busy ? "正在结算…" : "结算全部已入账抽成"}</button></div>
      </form>
      {entries.length === 0 ? <p className="empty">还没有分录。</p> : (
        <table>
          <thead><tr><th>时间</th><th>科目</th><th>方向</th><th>金额</th><th>状态</th><th>说明</th></tr></thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.id}>
                <td>{when(e.created_at)}</td>
                <td>{e.account}</td>
                <td>{e.direction === "debit" ? "借" : "贷"}</td>
                <td className="num">¥{yuan(e.amount_cents)}</td>
                <td>{e.status === "booked" ? "已入账" : e.status === "settled" ? "已结算" : "备忘"}</td>
                <td>{e.memo}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

export function AuditPage() {
  const loaded = useLoad(() => Promise.all([api.audit(), api.webhooks()]), []);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  async function password(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setErr("");
    setMsg("");
    try {
      await api.password(String(form.get("current")), String(form.get("next")));
      setMsg("管理员密码已更新。");
      e.currentTarget.reset();
    } catch (error) {
      setErr(error instanceof ApiError ? error.message : "没有改成");
    } finally { setBusy(false); }
  }
  const audits = loaded.data?.[0].entries ?? [];
  const hooks = loaded.data?.[1].deliveries ?? [];
  return (
    <>
      <header className="mast">
        <h1>审计</h1>
        <hr className="rule" />
        <p className="lede">登录、改价、买断、绑定和入账都会留在这里。下面是异步通知的投递结果，失败不会回滚已经入账的付款。</p>
      </header>
      <Banner>{loaded.error || err}</Banner>
      <Banner tone="ok">{msg}</Banner>
      {audits.length === 0 ? <p className="empty">还没有审计记录。</p> : (
        <table>
          <thead><tr><th>时间</th><th>谁</th><th>动作</th><th>对象</th></tr></thead>
          <tbody>
            {audits.map((a) => (
              <tr key={a.id}>
                <td>{when(a.created_at)}</td>
                <td>{a.actor_type}</td>
                <td>{a.action}</td>
                <td>{a.resource_type}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <section className="section">
        <h2>通知投递</h2>
        {hooks.length === 0 ? <p className="empty">还没有投递。项目没填通知地址时会记为跳过。</p> : (
          <table>
            <thead><tr><th>时间</th><th>事件</th><th>状态</th><th>地址</th><th>错误</th></tr></thead>
            <tbody>
              {hooks.map((h) => (
                <tr key={h.id}>
                  <td>{when(h.created_at)}</td>
                  <td>{h.event_type}</td>
                  <td>{h.status}</td>
                  <td>{h.target_url || "—"}</td>
                  <td>{h.last_error}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      <section className="section">
        <h2>更换管理员密码</h2>
        <form onSubmit={(e) => void password(e)}>
          <div className="grid-form">
            <div className="field"><label htmlFor="cur">当前密码</label><input id="cur" name="current" type="password" autoComplete="current-password" required /></div>
            <div className="field"><label htmlFor="next">新密码</label><input id="next" name="next" type="password" autoComplete="new-password" required minLength={8} /></div>
          </div>
          <div className="actions"><button className="primary" disabled={busy}>{busy ? "正在更新…" : "更新密码"}</button></div>
        </form>
      </section>
    </>
  );
}
