package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/yin/alipay-nowpay-gateway/internal/alipay"
	"github.com/yin/alipay-nowpay-gateway/internal/commission"
	"github.com/yin/alipay-nowpay-gateway/internal/domain"
	"github.com/yin/alipay-nowpay-gateway/internal/id"
	"github.com/yin/alipay-nowpay-gateway/internal/money"
	"github.com/yin/alipay-nowpay-gateway/internal/store"
	"github.com/yin/alipay-nowpay-gateway/internal/webhook"
	"golang.org/x/crypto/bcrypt"
)

type Actor struct {
	Type      string
	ID        string
	Email     string
	Name      string
	CreatorID string
	IP        string
}

type Service struct {
	DB     *store.Store
	Pay    alipay.Client
	Sim    alipay.Simulator
	Books  commission.Engine
	Secret string
	TTL    time.Duration
}

func New(db *store.Store, pay alipay.Client, sim alipay.Simulator, books commission.Engine, secret string, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &Service{DB: db, Pay: pay, Sim: sim, Books: books, Secret: secret, TTL: ttl}
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

func Public(err error) error {
	if err == nil {
		return nil
	}
	if domain.As(err) != nil {
		return err
	}
	if errors.Is(err, store.ErrNotFound) {
		return domain.E(404, "not_found", "没有找到记录")
	}
	if errors.Is(err, store.ErrInactive) {
		return domain.E(409, "project_inactive", "项目已停用，不能新开席位")
	}
	if errors.Is(err, store.ErrNothingToSettle) {
		return domain.E(409, "nothing_to_settle", "没有待结算的平台抽成")
	}
	if errors.Is(err, alipay.ErrNotReady) {
		return domain.E(503, "alipay_not_ready", alipay.ErrNotReady.Error())
	}
	var call *alipay.CallError
	if errors.As(err, &call) {
		return domain.E(409, call.SubCode, call.SubMsg)
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return domain.E(409, "conflict", "邮箱或编号已经存在")
	}
	slog.Error("request failed", "err", err)
	return domain.E(500, "internal", "系统繁忙，请稍后再试")
}

func (s *Service) Login(ctx context.Context, portal, email, password string) (string, Actor, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	portal = strings.ToLower(strings.TrimSpace(portal))
	var actor Actor
	var hash string
	switch portal {
	case "admin":
		admin, err := s.DB.AdminByEmail(ctx, email)
		if errors.Is(err, store.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return "", Actor{}, domain.E(401, "invalid_login", "邮箱或密码不正确")
		}
		if err != nil {
			return "", Actor{}, err
		}
		hash = admin.PasswordHash
		actor = Actor{Type: "admin", ID: admin.ID, Email: admin.Email, Name: admin.Name}
	case "creator":
		user, err := s.DB.PortalByEmail(ctx, email)
		if errors.Is(err, store.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return "", Actor{}, domain.E(401, "invalid_login", "邮箱或密码不正确")
		}
		if err != nil {
			return "", Actor{}, err
		}
		if user.CreatorStatus != "active" {
			return "", Actor{}, domain.E(403, "suspended", "这个创作者已停用")
		}
		hash = user.PasswordHash
		actor = Actor{Type: "creator", ID: user.UserID, Email: user.Email, Name: user.Name, CreatorID: user.CreatorID}
	default:
		return "", Actor{}, domain.E(400, "invalid_portal", "portal 需要是 admin 或 creator")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", Actor{}, domain.E(401, "invalid_login", "邮箱或密码不正确")
	}
	raw := id.Token(32)
	if err := s.DB.CreateSession(ctx, hashToken(raw), actor.Type, actor.ID, actor.CreatorID, time.Now().Add(s.TTL)); err != nil {
		return "", Actor{}, err
	}
	s.audit(ctx, actor, "auth.login", "session", actor.ID, map[string]string{"portal": portal})
	return raw, actor, nil
}

func (s *Service) Session(ctx context.Context, raw string) (Actor, error) {
	if strings.TrimSpace(raw) == "" {
		return Actor{}, domain.E(401, "unauthenticated", "请先登录")
	}
	row, err := s.DB.LookupSession(ctx, hashToken(raw))
	if errors.Is(err, store.ErrNotFound) {
		return Actor{}, domain.E(401, "unauthenticated", "登录已失效，请重新登录")
	}
	if err != nil {
		return Actor{}, err
	}
	if row.Type == "creator" && row.CreatorStatus != "active" {
		return Actor{}, domain.E(403, "suspended", "这个创作者已停用")
	}
	return Actor{Type: row.Type, ID: row.ID, Email: row.Email, Name: row.Name, CreatorID: row.CreatorID}, nil
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	return s.DB.DeleteSession(ctx, hashToken(raw))
}

func (s *Service) ChangeAdminPassword(ctx context.Context, a Actor, current, next string) error {
	if err := adminOnly(a); err != nil {
		return err
	}
	if err := checkPassword(next); err != nil {
		return err
	}
	hash, err := s.DB.AdminHash(ctx, a.ID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return domain.E(401, "invalid_login", "当前密码不正确")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.DB.UpdateAdminPassword(ctx, a.ID, string(hashed)); err != nil {
		return err
	}
	s.audit(ctx, a, "admin.password", "admin", a.ID, map[string]string{})
	return nil
}

type CreatorInput struct {
	Name           string `json:"name"`
	ContactEmail   string `json:"contact_email"`
	Phone          string `json:"phone"`
	Notes          string `json:"notes"`
	Status         string `json:"status"`
	PortalEmail    string `json:"portal_email"`
	PortalName     string `json:"portal_name"`
	PortalPassword string `json:"portal_password"`
}

func (s *Service) CreateCreator(ctx context.Context, a Actor, in CreatorInput) (store.Creator, error) {
	if err := adminOnly(a); err != nil {
		return store.Creator{}, err
	}
	name, err := checkName(in.Name, "创作者名称")
	if err != nil {
		return store.Creator{}, err
	}
	portalEmail, err := checkEmail(in.PortalEmail)
	if err != nil {
		return store.Creator{}, err
	}
	if err := checkPassword(in.PortalPassword); err != nil {
		return store.Creator{}, err
	}
	portalName := strings.TrimSpace(in.PortalName)
	if portalName == "" {
		portalName = name
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.PortalPassword), bcrypt.DefaultCost)
	if err != nil {
		return store.Creator{}, err
	}
	c := store.Creator{
		ID: id.New(), PublicCode: id.Code("cr_"), Name: name,
		ContactEmail: strings.TrimSpace(in.ContactEmail), Phone: strings.TrimSpace(in.Phone),
		Notes: strings.TrimSpace(in.Notes), Status: "active",
	}
	created, err := s.DB.CreateCreator(ctx, c, id.New(), portalEmail, portalName, string(hash))
	if err != nil {
		return store.Creator{}, err
	}
	s.audit(ctx, a, "creator.create", "creator", created.ID, map[string]string{"name": created.Name})
	return created, nil
}

func (s *Service) UpdateCreator(ctx context.Context, a Actor, creatorID string, in CreatorInput) (store.Creator, error) {
	if err := adminOnly(a); err != nil {
		return store.Creator{}, err
	}
	cur, err := s.DB.GetCreator(ctx, creatorID)
	if err != nil {
		return store.Creator{}, err
	}
	name, err := checkName(in.Name, "创作者名称")
	if err != nil {
		return store.Creator{}, err
	}
	status := in.Status
	if status == "" {
		status = cur.Status
	}
	if status != "active" && status != "suspended" {
		return store.Creator{}, domain.E(400, "invalid_status", "状态只能是 active 或 suspended")
	}
	cur.Name, cur.ContactEmail, cur.Phone, cur.Notes, cur.Status = name, strings.TrimSpace(in.ContactEmail), strings.TrimSpace(in.Phone), strings.TrimSpace(in.Notes), status
	if err := s.DB.UpdateCreator(ctx, cur); err != nil {
		return store.Creator{}, err
	}
	s.audit(ctx, a, "creator.update", "creator", creatorID, map[string]string{"status": status})
	return s.DB.GetCreator(ctx, creatorID)
}

func (s *Service) ResetCreatorPassword(ctx context.Context, a Actor, creatorID, password string) error {
	if err := adminOnly(a); err != nil {
		return err
	}
	if err := checkPassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.DB.SetCreatorPassword(ctx, creatorID, string(hash)); err != nil {
		return err
	}
	s.audit(ctx, a, "creator.password_reset", "creator", creatorID, map[string]string{})
	return nil
}

type ProjectInput struct {
	CreatorID         string
	Name              string
	Status            string
	CallbackURL       string
	NotifyURL         string
	BillingMode       string
	SeatPriceYuan     *string
	CustomerPriceYuan string
	CommissionPercent *string
}

func (s *Service) CreateProject(ctx context.Context, a Actor, in ProjectInput) (store.Project, error) {
	if err := adminOnly(a); err != nil {
		return store.Project{}, err
	}
	creator, err := s.DB.GetCreator(ctx, in.CreatorID)
	if err != nil {
		return store.Project{}, err
	}
	if creator.Status != "active" {
		return store.Project{}, domain.E(409, "creator_suspended", "创作者已停用")
	}
	p, err := s.normalizeProject(store.Project{ID: id.New(), PublicCode: id.Code("pj_"), CreatorID: creator.ID, Status: "active"}, in)
	if err != nil {
		return store.Project{}, err
	}
	created, err := s.DB.CreateProject(ctx, p)
	if err != nil {
		return store.Project{}, err
	}
	s.audit(ctx, a, "project.create", "project", created.ID, map[string]any{"name": created.Name, "creator_id": creator.ID})
	return created, nil
}

func (s *Service) UpdateProject(ctx context.Context, a Actor, projectID string, in ProjectInput) (store.Project, error) {
	if err := adminOnly(a); err != nil {
		return store.Project{}, err
	}
	cur, err := s.DB.GetProject(ctx, projectID)
	if err != nil {
		return store.Project{}, err
	}
	p, err := s.normalizeProject(cur, in)
	if err != nil {
		return store.Project{}, err
	}
	updated, err := s.DB.UpdateProject(ctx, p)
	if err != nil {
		return store.Project{}, err
	}
	s.audit(ctx, a, "project.update", "project", projectID, map[string]any{"name": updated.Name})
	return updated, nil
}

func (s *Service) UpdateEndpoints(ctx context.Context, a Actor, projectID, callback, notify string) (store.Project, error) {
	p, err := s.DB.GetProject(ctx, projectID)
	if err != nil {
		return store.Project{}, err
	}
	if err := canSee(a, p.CreatorID); err != nil {
		return store.Project{}, err
	}
	if err := checkURL(callback, true); err != nil {
		return store.Project{}, err
	}
	if err := checkURL(notify, true); err != nil {
		return store.Project{}, err
	}
	updated, err := s.DB.UpdateEndpoints(ctx, projectID, strings.TrimSpace(callback), strings.TrimSpace(notify))
	if err != nil {
		return store.Project{}, err
	}
	s.audit(ctx, a, "project.endpoints", "project", projectID, map[string]string{"callback_url": updated.CallbackURL})
	return updated, nil
}

func (s *Service) UpdateSeatPrice(ctx context.Context, a Actor, name, yuan string) error {
	if err := adminOnly(a); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "开通个人支付宝收款席位"
	}
	cents, err := money.ParseYuan(yuan)
	if err != nil || cents < 0 {
		return domain.E(400, "invalid_amount", "席位买断价格式应为元，最多两位小数")
	}
	if err := s.DB.UpdateOneTimePrice(ctx, name, cents); err != nil {
		return err
	}
	s.audit(ctx, a, "pricing.update", "price_catalog", "ONE_TIME", map[string]any{"amount_cents": cents, "name": name})
	return nil
}

func (s *Service) UpdateCommission(ctx context.Context, a Actor, rate, jsapi, nowpay string) error {
	if err := adminOnly(a); err != nil {
		return err
	}
	rb, err := money.PercentToBPS(rate)
	if err != nil {
		return domain.E(400, "invalid_rate", "平台抽成需要是 0 到 100 之间、最多两位小数的百分数")
	}
	jb, err := money.PercentToBPS(jsapi)
	if err != nil {
		return domain.E(400, "invalid_rate", "支付宝收单费率格式不正确")
	}
	nb, err := money.PercentToBPS(nowpay)
	if err != nil {
		return domain.E(400, "invalid_rate", "即刻收费率格式不正确")
	}
	settings := store.CommissionSettings{RateBPS: rb, AlipayJSAPIBPS: jb, AlipayNowPayBPS: nb}
	if err := s.DB.UpdateCommission(ctx, settings); err != nil {
		return err
	}
	s.audit(ctx, a, "commission.update", "commission_settings", "global", settings)
	return nil
}

func (s *Service) OpenSeat(ctx context.Context, a Actor, projectID string, payNow bool, channel string) (store.Seat, error) {
	p, err := s.DB.GetProject(ctx, projectID)
	if err != nil {
		return store.Seat{}, err
	}
	if err := canSee(a, p.CreatorID); err != nil {
		return store.Seat{}, err
	}
	creator, err := s.DB.GetCreator(ctx, p.CreatorID)
	if err != nil {
		return store.Seat{}, err
	}
	if creator.Status != "active" || p.Status != "active" {
		return store.Seat{}, domain.E(409, "inactive", "创作者或项目已停用")
	}
	if err := money.ValidatePhase1Buyout(p.BillingMode); err != nil {
		return store.Seat{}, domain.E(409, "billing_mode_reserved", err.Error())
	}
	if channel == "" {
		if a.Type == "creator" {
			channel = "mock_checkout"
		} else {
			channel = "admin_recorded"
		}
	}
	if channel != "mock_checkout" && channel != "admin_recorded" {
		return store.Seat{}, domain.E(400, "invalid_channel", "支付渠道无法识别")
	}
	seat, err := s.DB.CreateSeat(ctx, store.Seat{
		ID: id.New(), PublicCode: id.Code("seat_"), ProjectID: p.ID, CreatorID: p.CreatorID,
		BillingMode: money.BillingOneTime, AmountCents: p.EffectiveSeatPriceCents,
	}, channel)
	if err != nil {
		return store.Seat{}, err
	}
	s.audit(ctx, a, "seat.create", "seat", seat.ID, map[string]any{"ordinal": seat.Ordinal, "amount_cents": seat.AmountCents})
	if payNow || a.Type == "creator" {
		return s.PaySeat(ctx, a, seat.ID, channel)
	}
	return seat, nil
}

func (s *Service) PaySeat(ctx context.Context, a Actor, seatID, channel string) (store.Seat, error) {
	seat, err := s.DB.GetSeat(ctx, seatID)
	if err != nil {
		return store.Seat{}, err
	}
	if err := canSee(a, seat.CreatorID); err != nil {
		return store.Seat{}, err
	}
	if channel == "" {
		channel = "admin_recorded"
	}
	paid, err := s.DB.MarkSeatPaid(ctx, seatID, channel, "MOCK"+id.Token(6))
	if err != nil {
		return store.Seat{}, err
	}
	s.audit(ctx, a, "seat.pay", "seat", seatID, map[string]any{"channel": channel, "amount_cents": paid.AmountCents, "status": paid.Status})
	return paid, nil
}

func (s *Service) StartBind(ctx context.Context, a Actor, seatID string) (store.Seat, error) {
	seat, project, creator, err := s.seatContext(ctx, a, seatID)
	if err != nil {
		return store.Seat{}, err
	}
	if seat.Status == "pending_payment" {
		return store.Seat{}, domain.E(409, "seat_unpaid", "先完成席位买断，再绑定支付宝")
	}
	q, err := s.Pay.ChargeQuery(ctx, alipay.ChargeQueryBiz{OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode})
	if err != nil {
		return store.Seat{}, err
	}
	if err := failResult(q); err != nil {
		return store.Seat{}, err
	}
	var initBody *alipay.ChargeInitializeBody
	if q.Body.CapabilityStatus == alipay.StatusNotConfigured || q.Body.CapabilityStatus == alipay.StatusConfiguring {
		callback := project.CallbackURL
		res, err := s.Pay.ChargeInitialize(ctx, alipay.ChargeInitializeBiz{
			OutProductID:    seat.PublicCode,
			ExternalOwnerID: creator.PublicCode,
			ProductName:     trimRunes(project.Name+" · 收款席位", 120),
			CallbackURL:     callback,
			PricingModeList: []alipay.PricingMode{{BillingMode: money.BillingOneTime}},
		})
		if err != nil {
			return store.Seat{}, err
		}
		if !res.OK && res.SubCode == "NOW_PAY_ALREADY_CONFIGURED" {
			q, err = s.Pay.ChargeQuery(ctx, alipay.ChargeQueryBiz{OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode})
			if err != nil {
				return store.Seat{}, err
			}
			if err := failResult(q); err != nil {
				return store.Seat{}, err
			}
		} else if err := failResult(res); err != nil {
			return store.Seat{}, err
		} else {
			initBody = &res.Body
			q, err = s.Pay.ChargeQuery(ctx, alipay.ChargeQueryBiz{OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode})
			if err != nil {
				return store.Seat{}, err
			}
		}
	}
	if err := s.persistCapability(ctx, seat, creator.PublicCode, q.Body, initBody); err != nil {
		return store.Seat{}, err
	}
	s.audit(ctx, a, "seat.bind_start", "seat", seat.ID, map[string]string{"capability_status": q.Body.CapabilityStatus})
	return s.DB.GetSeat(ctx, seat.ID)
}

func (s *Service) CompleteBind(ctx context.Context, a Actor, seatID, priceYuan, logon string) (store.Seat, error) {
	if s.Sim == nil {
		return store.Seat{}, domain.E(409, "mock_only", "当前不是模拟网关，不能在控制台里代替创作者完成支付宝授权")
	}
	seat, project, creator, err := s.seatContext(ctx, a, seatID)
	if err != nil {
		return store.Seat{}, err
	}
	if seat.Status == "pending_payment" {
		return store.Seat{}, domain.E(409, "seat_unpaid", "先完成席位买断，再绑定支付宝")
	}
	if strings.TrimSpace(priceYuan) == "" {
		priceYuan = money.FormatYuan(project.CustomerPriceCents)
	}
	done, err := s.Sim.CompleteConfiguration(ctx, alipay.CompleteConfigInput{
		OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode, PriceYuan: priceYuan, LogonID: logon, DisplayName: "永久买断",
	})
	if err != nil {
		return store.Seat{}, err
	}
	if err := failResult(done); err != nil {
		return store.Seat{}, err
	}
	q, err := s.Pay.ChargeQuery(ctx, alipay.ChargeQueryBiz{OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode})
	if err != nil {
		return store.Seat{}, err
	}
	if err := failResult(q); err != nil {
		return store.Seat{}, err
	}
	if err := s.persistCapability(ctx, seat, creator.PublicCode, q.Body, nil); err != nil {
		return store.Seat{}, err
	}
	updated, err := s.DB.GetSeat(ctx, seat.ID)
	if err != nil {
		return store.Seat{}, err
	}
	s.audit(ctx, a, "seat.bind_complete", "seat", seat.ID, map[string]any{"capability_status": updated.CapabilityStatus, "logon": updated.AlipayLogonMasked})
	if updated.Status == "bound" {
		s.notify(ctx, project, "charge.enabled", map[string]any{"seat_id": updated.ID, "capability_status": updated.CapabilityStatus})
	}
	return updated, nil
}

func (s *Service) CompleteByTicket(ctx context.Context, ticket, priceYuan, logon string) (store.Seat, error) {
	if s.Sim == nil {
		return store.Seat{}, domain.E(409, "mock_only", "模拟托管页只在 ALIPAY_MODE=mock 时可用")
	}
	view, err := s.Sim.LookupTicket(ctx, ticket)
	if err != nil {
		return store.Seat{}, err
	}
	if view.Kind != "config" {
		return store.Seat{}, domain.E(404, "not_found", "这不是收费配置页")
	}
	seat, err := s.DB.SeatByCode(ctx, view.OutProductID)
	if err != nil {
		return store.Seat{}, err
	}
	return s.CompleteBind(ctx, Actor{Type: "system"}, seat.ID, priceYuan, logon)
}

func (s *Service) ModifyCapability(ctx context.Context, a Actor, seatID, action string) (store.Seat, error) {
	if err := adminOnly(a); err != nil {
		return store.Seat{}, err
	}
	seat, _, creator, err := s.seatContext(ctx, a, seatID)
	if err != nil {
		return store.Seat{}, err
	}
	res, err := s.Pay.ChargeModify(ctx, alipay.ChargeModifyBiz{OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode, Action: action})
	if err != nil {
		return store.Seat{}, err
	}
	if err := failResult(res); err != nil {
		return store.Seat{}, err
	}
	q, err := s.Pay.ChargeQuery(ctx, alipay.ChargeQueryBiz{OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode})
	if err != nil {
		return store.Seat{}, err
	}
	if err := failResult(q); err != nil {
		return store.Seat{}, err
	}
	if err := s.persistCapability(ctx, seat, creator.PublicCode, q.Body, nil); err != nil {
		return store.Seat{}, err
	}
	s.audit(ctx, a, "seat.capability", "seat", seatID, map[string]any{"action": action, "changed": res.Body.Changed})
	return s.DB.GetSeat(ctx, seatID)
}

func (s *Service) SimulatePurchase(ctx context.Context, a Actor, seatID, buyer string) (store.Purchase, error) {
	p, err := s.CreatePurchaseLink(ctx, a, seatID, buyer)
	if err != nil {
		return store.Purchase{}, err
	}
	return s.PayPurchase(ctx, a, p.OutRequestNo)
}

func (s *Service) CreatePurchaseLink(ctx context.Context, a Actor, seatID, buyer string) (store.Purchase, error) {
	seat, project, creator, err := s.seatContext(ctx, a, seatID)
	if err != nil {
		return store.Purchase{}, err
	}
	if project.Status != "active" || creator.Status != "active" {
		return store.Purchase{}, domain.E(409, "inactive", "项目或创作者已停用")
	}
	if seat.Status != "bound" {
		return store.Purchase{}, domain.E(409, "seat_unbound", "这个席位还没有绑定可收款的支付宝")
	}
	buyer, err = normalizeBuyer(buyer)
	if err != nil {
		return store.Purchase{}, err
	}
	q, err := s.Pay.ChargeQuery(ctx, alipay.ChargeQueryBiz{OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode})
	if err != nil {
		return store.Purchase{}, err
	}
	if err := failResult(q); err != nil {
		return store.Purchase{}, err
	}
	if q.Body.CapabilityStatus != alipay.StatusEnabled || len(q.Body.ChargingOptions) == 0 {
		return store.Purchase{}, domain.E(409, "not_purchasable", "收费能力未启用，或还没有可售档位")
	}
	opt := q.Body.ChargingOptions[0]
	gross, err := money.ParseYuan(opt.Price)
	if err != nil {
		return store.Purchase{}, domain.E(409, "invalid_amount", "档位价格无法入账")
	}
	reqNo := "np_" + id.Token(8)
	created, err := s.Pay.PurchaseCreate(ctx, alipay.PurchaseCreateBiz{
		OutRequestNo: reqNo, OutProductID: seat.PublicCode, ExternalOwnerID: creator.PublicCode,
		ExternalBuyerID: buyer, SKUID: opt.SKUID, CallbackURL: project.CallbackURL,
	})
	if err != nil {
		return store.Purchase{}, err
	}
	if err := failResult(created); err != nil {
		return store.Purchase{}, err
	}
	var exp *time.Time
	if t, err := alipay.ParseShanghai(created.Body.ExpireTime); err == nil {
		exp = &t
	}
	row := store.Purchase{
		ID: id.New(), SeatID: seat.ID, ProjectID: project.ID, CreatorID: creator.ID,
		OutRequestNo: reqNo, ExternalBuyerID: buyer, SKUID: opt.SKUID, GrossCents: gross,
		PurchaseURL: created.Body.PurchaseURL, PurchaseQRCode: created.Body.PurchaseQRCode, ExpireAt: exp,
	}
	if err := s.DB.InsertOpenPurchase(ctx, row); err != nil {
		return store.Purchase{}, err
	}
	return s.DB.PurchaseByRequest(ctx, reqNo)
}

func (s *Service) PayPurchase(ctx context.Context, a Actor, outRequestNo string) (store.Purchase, error) {
	if s.Sim == nil {
		return store.Purchase{}, domain.E(409, "mock_only", "正式网关不会在这里模拟顾客付款")
	}
	paid, err := s.Sim.MarkPurchasePaid(ctx, outRequestNo)
	if err != nil {
		return store.Purchase{}, err
	}
	seat, err := s.DB.SeatByCode(ctx, paid.OutProductID)
	if err != nil {
		return store.Purchase{}, err
	}
	if a.Type != "system" && a.Type != "" {
		if err := canSee(a, seat.CreatorID); err != nil {
			return store.Purchase{}, err
		}
	}
	project, err := s.DB.GetProject(ctx, seat.ProjectID)
	if err != nil {
		return store.Purchase{}, err
	}
	settings, err := s.DB.Commission(ctx)
	if err != nil {
		return store.Purchase{}, err
	}
	plan, err := s.Books.Plan(ctx, commission.Input{
		GrossCents: paid.GrossCents, CommissionBPS: project.EffectiveCommissionBPS,
		AlipayJSAPIBPS: settings.AlipayJSAPIBPS, AlipayNowPayBPS: settings.AlipayNowPayBPS,
	})
	if err != nil {
		return store.Purchase{}, domain.E(400, "commission", err.Error())
	}
	now := time.Now()
	lines := make([]store.LedgerLine, 0, len(plan.Lines))
	for _, ln := range plan.Lines {
		lines = append(lines, store.LedgerLine{Account: ln.Account, Direction: ln.Direction, Amount: ln.Amount, Status: ln.Status, Memo: ln.Memo})
	}
	row := store.Purchase{
		SeatID: seat.ID, ProjectID: project.ID, CreatorID: seat.CreatorID,
		OutRequestNo: outRequestNo, ExternalBuyerID: paid.ExternalBuyerID, SKUID: paid.SKUID,
		GrossCents: paid.GrossCents, AlipayTradeNo: paid.TradeNo,
		CommissionCents: plan.CommissionCents, CommissionBPS: project.EffectiveCommissionBPS,
		AlipayFeeCents: plan.AlipayFeeCents, CreatorNetCents: plan.CreatorNetCents,
		EstimatesExceedGross: plan.EstimatesExceedGross, PaidAt: &now,
	}
	booked, fresh, err := s.DB.BookPaid(ctx, row, lines)
	if err != nil {
		return store.Purchase{}, err
	}
	consult, cerr := s.Pay.PurchaseConsult(ctx, alipay.PurchaseConsultBiz{
		OutProductID: seat.PublicCode, ExternalOwnerID: paid.ExternalOwnerID, ExternalBuyerID: paid.ExternalBuyerID,
		CallbackURL: project.CallbackURL,
	})
	if cerr == nil && consult.OK {
		_ = s.DB.SetDecision(ctx, outRequestNo, consult.Body.Decision)
		booked.Decision = consult.Body.Decision
	}
	if fresh {
		s.audit(ctx, a, "purchase.paid", "purchase", booked.ID, map[string]any{
			"gross_cents": booked.GrossCents, "commission_cents": booked.CommissionCents, "buyer": booked.ExternalBuyerID,
		})
		s.notify(ctx, project, "purchase.paid", booked)
	}
	return booked, nil
}

func (s *Service) PayByTicket(ctx context.Context, ticket string) (store.Purchase, error) {
	if s.Sim == nil {
		return store.Purchase{}, domain.E(409, "mock_only", "模拟付款页只在 ALIPAY_MODE=mock 时可用")
	}
	view, err := s.Sim.LookupTicket(ctx, ticket)
	if err != nil {
		return store.Purchase{}, err
	}
	if view.Kind != "purchase" {
		return store.Purchase{}, domain.E(404, "not_found", "这不是购买页")
	}
	if view.Expired && !view.Paid {
		return store.Purchase{}, domain.E(409, "NOW_PAY_PURCHASE_LINK_INVALID", "购买链接已过期")
	}
	return s.PayPurchase(ctx, Actor{Type: "system"}, view.OutRequestNo)
}

func (s *Service) LookupTicket(ctx context.Context, ticket string) (alipay.TicketView, error) {
	if s.Sim == nil {
		return alipay.TicketView{}, domain.E(409, "mock_only", "模拟托管页只在 ALIPAY_MODE=mock 时可用")
	}
	return s.Sim.LookupTicket(ctx, ticket)
}

func (s *Service) Settle(ctx context.Context, a Actor, note string) (store.Settlement, error) {
	if err := adminOnly(a); err != nil {
		return store.Settlement{}, err
	}
	batch, err := s.DB.Settle(ctx, strings.TrimSpace(note), s.Books.Name())
	if err != nil {
		return store.Settlement{}, err
	}
	s.audit(ctx, a, "settlement.create", "settlement", batch.ID, map[string]any{"total_cents": batch.TotalCents, "engine": batch.Engine})
	return batch, nil
}

func (s *Service) seatContext(ctx context.Context, a Actor, seatID string) (store.Seat, store.Project, store.Creator, error) {
	seat, err := s.DB.GetSeat(ctx, seatID)
	if err != nil {
		return store.Seat{}, store.Project{}, store.Creator{}, err
	}
	if a.Type != "system" {
		if err := canSee(a, seat.CreatorID); err != nil {
			return store.Seat{}, store.Project{}, store.Creator{}, err
		}
	}
	project, err := s.DB.GetProject(ctx, seat.ProjectID)
	if err != nil {
		return store.Seat{}, store.Project{}, store.Creator{}, err
	}
	creator, err := s.DB.GetCreator(ctx, seat.CreatorID)
	if err != nil {
		return store.Seat{}, store.Project{}, store.Creator{}, err
	}
	return seat, project, creator, nil
}

func (s *Service) persistCapability(ctx context.Context, seat store.Seat, owner string, body alipay.ChargeQueryBody, initBody *alipay.ChargeInitializeBody) error {
	raw, _ := json.Marshal(body.ChargingOptions)
	status := seat.Status
	markBound := seat.Status == "bound"
	switch body.CapabilityStatus {
	case alipay.StatusEnabled, alipay.StatusDisabled:
		if seat.Status != "pending_payment" {
			status = "bound"
			markBound = true
		}
	case alipay.StatusConfiguring, alipay.StatusNotConfigured:
		if seat.Status == "paid_unbound" || seat.Status == "configuring" {
			status = "configuring"
		}
	}
	userID, logon := seat.AlipayUserID, seat.AlipayLogonMasked
	if s.Sim != nil && (body.CapabilityStatus == alipay.StatusEnabled || body.CapabilityStatus == alipay.StatusDisabled) {
		if uid, masked, err := s.Sim.Account(ctx, seat.PublicCode, owner); err == nil {
			if uid != "" {
				userID = uid
			}
			if masked != "" {
				logon = masked
			}
		}
	}
	cap := store.Capability{
		SeatStatus: status, CapabilityStatus: body.CapabilityStatus,
		AlipayUserID: userID, LogonMasked: logon,
		ConfigurationURL: seat.ConfigurationURL, ConfigurationQRCode: seat.ConfigurationQRCode,
		ManagementURL: body.ManagementURL, BillingModeActual: body.BillingMode,
		Options: raw, StatusVersion: body.StatusVersion, MarkBound: markBound,
	}
	if initBody != nil {
		if initBody.ConfigurationURL != "" {
			cap.ConfigurationURL = initBody.ConfigurationURL
		}
		if initBody.ConfigurationQRCode != "" {
			cap.ConfigurationQRCode = initBody.ConfigurationQRCode
		}
		if initBody.ManagementURL != "" && cap.ManagementURL == "" {
			cap.ManagementURL = initBody.ManagementURL
		}
	}
	if cap.ManagementURL == "" {
		cap.ManagementURL = seat.ManagementURL
	}
	return s.DB.SaveCapability(ctx, seat.ID, cap)
}

func (s *Service) normalizeProject(base store.Project, in ProjectInput) (store.Project, error) {
	name, err := checkName(in.Name, "项目名称")
	if err != nil {
		return store.Project{}, err
	}
	mode := in.BillingMode
	if mode == "" {
		mode = money.BillingOneTime
	}
	if err := money.ValidatePhase1Buyout(mode); err != nil {
		return store.Project{}, domain.E(409, "billing_mode_reserved", err.Error())
	}
	if err := checkURL(in.CallbackURL, true); err != nil || checkURL(in.NotifyURL, true) != nil {
		if err == nil {
			err = checkURL(in.NotifyURL, true)
		}
		return store.Project{}, err
	}
	seatPrice, err := optionalYuan(in.SeatPriceYuan)
	if err != nil {
		return store.Project{}, err
	}
	customer, err := money.ParseYuan(in.CustomerPriceYuan)
	if err != nil || customer < 1 {
		return store.Project{}, domain.E(400, "invalid_amount", "顾客永久买断价至少 0.01 元")
	}
	commissionBPS, err := optionalPercent(in.CommissionPercent)
	if err != nil {
		return store.Project{}, err
	}
	status := in.Status
	if status == "" {
		status = base.Status
	}
	if status != "active" && status != "suspended" {
		return store.Project{}, domain.E(400, "invalid_status", "状态只能是 active 或 suspended")
	}
	base.Name = name
	base.Status = status
	base.CallbackURL = strings.TrimSpace(in.CallbackURL)
	base.NotifyURL = strings.TrimSpace(in.NotifyURL)
	base.BillingMode = mode
	base.SeatPriceCents = seatPrice
	base.CustomerPriceCents = customer
	base.CommissionBPS = commissionBPS
	return base, nil
}

func (s *Service) audit(ctx context.Context, a Actor, action, kind, resource string, detail any) {
	if a.Type == "" {
		a.Type = "system"
	}
	b, err := json.Marshal(detail)
	if err != nil {
		b = []byte(`{}`)
	}
	if err := s.DB.WriteAudit(ctx, store.Audit{
		ActorType: a.Type, ActorID: a.ID, Action: action, ResourceType: kind, ResourceID: resource, Detail: b, IP: a.IP,
	}); err != nil {
		slog.Error("audit", "err", err, "action", action)
	}
}

func (s *Service) notify(ctx context.Context, project store.Project, event string, data any) {
	body, err := json.Marshal(map[string]any{
		"id": id.New(), "type": event, "created_at": time.Now().UTC().Format(time.RFC3339), "data": data,
	})
	if err != nil {
		return
	}
	res := webhook.Deliver(ctx, s.Secret, project.NotifyURL, event, body)
	if err := s.DB.InsertWebhook(ctx, store.Webhook{
		ID: id.New(), ProjectID: project.ID, EventType: event, TargetURL: project.NotifyURL,
		Payload: body, Status: res.Status, Attempts: res.Attempts, LastError: res.LastError,
	}); err != nil {
		slog.Error("webhook", "err", err)
	}
}

func failResult[T any](res alipay.Result[T]) error {
	if res.OK {
		return nil
	}
	code, msg := res.SubCode, res.SubMsg
	if code == "" {
		code = res.Code
	}
	if msg == "" {
		msg = res.Msg
	}
	return domain.E(409, code, msg)
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func adminOnly(a Actor) error {
	if a.Type != "admin" {
		return domain.E(403, "forbidden", "需要管理员登录")
	}
	return nil
}

func canSee(a Actor, creatorID string) error {
	if a.Type == "admin" || (a.Type == "creator" && a.CreatorID == creatorID) {
		return nil
	}
	return domain.E(403, "forbidden", "不能操作其他创作者的项目")
}

func checkName(s, label string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 64 {
		return "", domain.E(400, "invalid_name", label+"需要 1 到 64 个字")
	}
	return s, nil
}

func checkEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.Contains(s, "@") || strings.Contains(s, " ") || len(s) > 254 {
		return "", domain.E(400, "invalid_email", "邮箱格式不正确")
	}
	return s, nil
}

func checkPassword(p string) error {
	if len(p) < 8 || len(p) > 72 {
		return domain.E(400, "weak_password", "密码需要 8 到 72 个字符")
	}
	return nil
}

func checkURL(raw string, allowEmpty bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" && allowEmpty {
		return nil
	}
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "http://") || strings.Contains(raw, " ") || len(raw) > 1024 {
		return domain.E(400, "invalid_url", "地址需要是完整的 http 或 https URL")
	}
	return nil
}

func optionalYuan(s *string) (*int64, error) {
	if s == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil, nil
	}
	v, err := money.ParseYuan(t)
	if err != nil {
		return nil, domain.E(400, "invalid_amount", "金额格式应为元，最多两位小数")
	}
	return &v, nil
}

func optionalPercent(s *string) (*int, error) {
	if s == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil, nil
	}
	v, err := money.PercentToBPS(t)
	if err != nil {
		return nil, domain.E(400, "invalid_rate", "抽成比例需要是 0 到 100 之间、最多两位小数的百分数")
	}
	return &v, nil
}

func normalizeBuyer(in string) (string, error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "buyer_" + id.Token(4), nil
	}
	if len(in) > 128 || strings.Contains(in, " ") {
		return "", domain.E(400, "invalid_buyer", "买家标识过长或包含空格")
	}
	return in, nil
}

func trimRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}
