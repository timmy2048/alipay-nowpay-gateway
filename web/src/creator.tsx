import { FormEvent, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ApiError, api } from "./api";
import { capLabel, seatLabel, when, yuan } from "./format";
import { Banner, Dot, QR } from "./ui";

export function CreatorHome() {
  const [projects, setProjects] = useState<Awaited<ReturnType<typeof api.creatorProjects>> | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    api.creatorProjects().then(setProjects).catch((err) => setError(err instanceof ApiError ? err.message : "没有读到项目"));
  }, []);
  return (
    <>
      <header className="mast">
        <h1>我的项目</h1>
        <hr className="rule" />
        <p className="lede">管理员已经把项目建好。你要做的是买断席位、用支付宝授权，并填好付款之后回到哪里。</p>
      </header>
      <Banner>{error}</Banner>
      {!projects || projects.projects.length === 0 ? <p className="empty">还没有分给你的项目。</p> : (
        <table>
          <thead><tr><th>项目</th><th>再开一席</th><th>已绑定</th></tr></thead>
          <tbody>
            {projects.projects.map((p) => (
              <tr key={p.id}>
                <td><Link to={`/creator/projects/${p.id}`}>{p.name}</Link></td>
                <td className="num">¥{yuan(p.effective_seat_price_cents)}</td>
                <td>{p.bound_count}/{p.seat_count}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

export function CreatorProject() {
  const { id = "" } = useParams();
  const [desk, setDesk] = useState<Awaited<ReturnType<typeof api.creatorDesk>> | null>(null);
  const [error, setError] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState("");
  async function load() {
    const row = await api.creatorDesk(id);
    setDesk(row);
  }
  useEffect(() => {
    load().catch((err) => setError(err instanceof ApiError ? err.message : "没有打开项目"));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);
  async function run(key: string, fn: () => Promise<unknown>, okText: string) {
    setBusy(key);
    setError("");
    setNote("");
    try {
      await fn();
      setNote(okText);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "没有完成");
    } finally { setBusy(""); }
  }
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    await run("save", () => api.saveEndpoints(id, String(form.get("callback_url") ?? ""), String(form.get("notify_url") ?? "")), "回跳地址已保存。");
  }
  const project = desk?.project;
  return (
    <>
      <header className="mast">
        <h1>{project?.name ?? "项目"}</h1>
        <hr className="rule" />
        <p className="lede">{project ? `再绑定一个支付宝，需要再买断 ¥${yuan(project.effective_seat_price_cents)}。这是付给平台的席位费，不是顾客付给你的钱。` : "正在打开。"}</p>
      </header>
      <Banner>{error}</Banner>
      <Banner tone="ok">{note}</Banner>
      {project && (
        <form key={project.id} onSubmit={(e) => void save(e)}>
          <div className="grid-form">
            <div className="field"><label htmlFor="callback">顾客付完款回到</label><input id="callback" name="callback_url" defaultValue={project.callback_url} placeholder="https://" /></div>
            <div className="field"><label htmlFor="notify">服务端通知地址</label><input id="notify" name="notify_url" defaultValue={project.notify_url} placeholder="可留空" /></div>
          </div>
          <div className="actions">
            <button className="primary" disabled={busy !== ""}>{busy === "save" ? "正在保存…" : "保存地址"}</button>
            <button type="button" disabled={busy !== ""} onClick={() => void run("buy", () => api.checkout(project.id), "席位已买断。下一步是支付宝授权。")}>
              {busy === "buy" ? "正在登记…" : `买断一个新席位 ¥${yuan(project.effective_seat_price_cents)}`}
            </button>
          </div>
        </form>
      )}
      <section className="section">
        <h2>席位</h2>
        {!desk || desk.seats.length === 0 ? <p className="empty">还没有席位。</p> : desk.seats.map((seat) => (
          <article key={seat.id} className="section">
            <h2>第 {seat.ordinal} 席</h2>
            <p className="hint"><Dot kind={seat.status}>{seatLabel(seat.status)}</Dot> · <Dot kind={seat.capability_status}>{capLabel(seat.capability_status)}</Dot>{seat.alipay_logon_masked ? ` · ${seat.alipay_logon_masked}` : ""}</p>
            {seat.status !== "pending_payment" && seat.status !== "bound" && (
              <div className="actions">
                <button type="button" disabled={busy !== ""} onClick={() => void run(seat.id, () => api.creatorBind(seat.id), "二维码已生成。")}>生成授权二维码</button>
                <button type="button" className="primary" disabled={busy !== ""} onClick={() => void run(seat.id + "c", () => api.creatorComplete(seat.id, "", ""), "已模拟完成授权。")}>我已在支付宝授权</button>
              </div>
            )}
            {seat.configuration_qr_code && seat.status !== "bound" && (
              <div className="qr">
                <QR value={seat.configuration_qr_code} />
                <p className="hint"><a href={seat.configuration_qr_code}>打开模拟支付宝</a>，确认登录号并完成授权。正式环境请用支付宝扫码。</p>
              </div>
            )}
          </article>
        ))}
      </section>
      <section className="section">
        <h2>顾客付给你的</h2>
        {!desk || desk.purchases.length === 0 ? <p className="empty">还没有顾客付款。钱会进你绑定的支付宝，平台只记账上的抽成。</p> : (
          <table>
            <thead><tr><th>时间</th><th>买家</th><th>实付</th><th>平台抽成</th><th>约计到账</th></tr></thead>
            <tbody>
              {desk.purchases.filter((p) => p.status === "paid").map((p) => (
                <tr key={p.id}>
                  <td>{when(p.paid_at)}</td>
                  <td>{p.external_buyer_id}</td>
                  <td className="num">¥{yuan(p.gross_cents)}</td>
                  <td className="num">¥{yuan(p.commission_cents)}</td>
                  <td className="num">¥{yuan(p.creator_net_cents)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}
