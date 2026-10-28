import { FormEvent, useEffect, useState } from "react";
import { NavLink, Navigate, Outlet, Route, Routes, useNavigate } from "react-router-dom";
import { Actor, ApiError, api } from "./api";
import { AuditPage, CreatorsPage, DashboardPage, DeskPage, LedgerPage, PricingPage, ProjectsPage } from "./admin";
import { CreatorHome, CreatorProject } from "./creator";
import { MockConfig, MockPurchase } from "./mock";
import { Banner } from "./ui";

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/mock/alipay/config" element={<MockConfig />} />
      <Route path="/mock/alipay/purchase" element={<MockPurchase />} />
      <Route element={<Guard portal="admin" />}>
        <Route path="/admin" element={<Shell portal="admin" />}>
          <Route index element={<DashboardPage />} />
          <Route path="creators" element={<CreatorsPage />} />
          <Route path="projects" element={<ProjectsPage />} />
          <Route path="projects/:id" element={<DeskPage />} />
          <Route path="pricing" element={<PricingPage />} />
          <Route path="ledger" element={<LedgerPage />} />
          <Route path="audit" element={<AuditPage />} />
        </Route>
      </Route>
      <Route element={<Guard portal="creator" />}>
        <Route path="/creator" element={<Shell portal="creator" />}>
          <Route index element={<CreatorHome />} />
          <Route path="projects/:id" element={<CreatorProject />} />
        </Route>
      </Route>
      <Route path="*" element={<Start />} />
    </Routes>
  );
}

function Start() {
  const [actor, setActor] = useState<Actor | null | undefined>(undefined);
  useEffect(() => {
    api.me().then(setActor).catch(() => setActor(null));
  }, []);
  if (actor === undefined) return <p className="main">正在读取账册…</p>;
  if (!actor) return <Navigate to="/login" replace />;
  return <Navigate to={actor.actor_type === "admin" ? "/admin" : "/creator"} replace />;
}

function Guard({ portal }: { portal: string }) {
  const [actor, setActor] = useState<Actor | null | undefined>(undefined);
  useEffect(() => {
    api.me().then(setActor).catch(() => setActor(null));
  }, []);
  if (actor === undefined) return <p className="main">正在读取账册…</p>;
  if (!actor) return <Navigate to="/login" replace />;
  if (actor.actor_type !== portal) return <Navigate to={actor.actor_type === "admin" ? "/admin" : "/creator"} replace />;
  return <Outlet context={actor} />;
}

function Shell({ portal }: { portal: "admin" | "creator" }) {
  const navigate = useNavigate();
  const [mode, setMode] = useState("mock");
  useEffect(() => {
    api.health().then((h) => setMode(h.alipay_mode)).catch(() => setMode(""));
  }, []);
  async function logout() {
    await api.logout();
    navigate("/login");
  }
  const admin = portal === "admin";
  return (
    <div className="app">
      <a className="skip" href="#content">跳到内容</a>
      <aside className="sidebar">
        <p className="brand">即刻收<span>{admin ? "平台账房" : "创作者办事"}</span></p>
        <nav className="nav" aria-label={admin ? "管理" : "创作者"}>
          {admin ? (
            <>
              <NavLink to="/admin" end>今日账</NavLink>
              <NavLink to="/admin/creators">创作者</NavLink>
              <NavLink to="/admin/projects">项目</NavLink>
              <NavLink to="/admin/pricing">席位与抽成</NavLink>
              <NavLink to="/admin/ledger">分录</NavLink>
              <NavLink to="/admin/audit">审计</NavLink>
            </>
          ) : (
            <NavLink to="/creator" end>我的项目</NavLink>
          )}
        </nav>
        <div className="side-foot">
          <p>{mode === "live" ? "正式适配器未接通" : "模拟网关 · 阶段一"}</p>
          <button type="button" onClick={() => void logout()}>退出</button>
        </div>
      </aside>
      <main className="main" id="content">
        <Outlet />
      </main>
    </div>
  );
}

function Login() {
  const navigate = useNavigate();
  const [portal, setPortal] = useState("admin");
  const [email, setEmail] = useState("admin@yin.local");
  const [password, setPassword] = useState("");
  const [show, setShow] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const actor = await api.login(email, password, portal);
      navigate(actor.actor_type === "admin" ? "/admin" : "/creator");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "登录没有完成");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login">
      <form onSubmit={onSubmit}>
        <h1>进入账房</h1>
        <p className="hint">YIN 的即刻收平台账房。创作者买断席位，顾客的钱进创作者的支付宝。</p>
        <Banner>{error}</Banner>
        <div className="choice" role="radiogroup" aria-label="入口">
          <label><input type="radio" name="portal" checked={portal === "admin"} onChange={() => { setPortal("admin"); setEmail("admin@yin.local"); }} /> 管理员</label>
          <label><input type="radio" name="portal" checked={portal === "creator"} onChange={() => { setPortal("creator"); setEmail(""); }} /> 创作者</label>
        </div>
        <div className="field">
          <label htmlFor="email">邮箱</label>
          <input id="email" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="field" style={{ marginTop: "0.8rem" }}>
          <label htmlFor="password">密码</label>
          <input id="password" type={show ? "text" : "password"} autoComplete="current-password" required minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <p className="hint">
          <button type="button" className="quiet" aria-pressed={show} onClick={() => setShow((v) => !v)}>{show ? "隐藏密码" : "显示密码"}</button>
        </p>
        <div className="actions">
          <button className="primary" type="submit" disabled={busy}>{busy ? "正在核对…" : "登录"}</button>
        </div>
        <p className="hint">本地 compose 的管理员是 admin@yin.local，初始密码见 README 的 ADMIN_PASSWORD（示例为 changeme-yin）。登录后请立刻改掉。</p>
        <p className="hint"><a href="/api/openapi.yaml">内部 API 文档</a></p>
      </form>
    </div>
  );
}
