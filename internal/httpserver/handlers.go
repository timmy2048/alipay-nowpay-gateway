package httpserver

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yin/alipay-nowpay-gateway/internal/domain"
	"github.com/yin/alipay-nowpay-gateway/internal/service"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DB.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "down", "alipay_mode": s.mode})
		return
	}
	ok(w, map[string]any{
		"status":            "ok",
		"alipay_mode":       s.mode,
		"commission_engine": s.svc.Books.Name(),
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Portal   string `json:"portal"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	raw, actor, err := s.svc.Login(r.Context(), body.Portal, body.Email, body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	actor.IP = clientIP(r)
	s.setSession(w, raw)
	writeJSON(w, http.StatusOK, publicActor(actor))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	_ = s.svc.Logout(r.Context(), tokenFrom(r))
	s.clearSession(w)
	ok(w, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	ok(w, publicActor(current(r)))
}

func publicActor(a service.Actor) map[string]any {
	return map[string]any{
		"actor_type": a.Type, "id": a.ID, "email": a.Email, "name": a.Name, "creator_id": a.CreatorID,
	}
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.svc.DB.Dashboard(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, d)
}

func (s *Server) listCreators(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListCreators(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"creators": rows})
}

func (s *Server) createCreator(w http.ResponseWriter, r *http.Request) {
	var body service.CreatorInput
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.svc.CreateCreator(r.Context(), current(r), body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) getCreator(w http.ResponseWriter, r *http.Request) {
	row, err := s.svc.DB.GetCreator(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) updateCreator(w http.ResponseWriter, r *http.Request) {
	var body service.CreatorInput
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.svc.UpdateCreator(r.Context(), current(r), chi.URLParam(r, "id"), body)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) resetCreatorPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.svc.ResetCreatorPassword(r.Context(), current(r), chi.URLParam(r, "id"), body.Password); err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListProjects(r.Context(), r.URL.Query().Get("creator_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"projects": rows})
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	in, err := readProject(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.svc.CreateProject(r.Context(), current(r), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	s.writeDesk(w, r, chi.URLParam(r, "id"), "")
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	in, err := readProject(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.svc.UpdateProject(r.Context(), current(r), chi.URLParam(r, "id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) createSeat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PayNow  bool   `json:"pay_now"`
		Channel string `json:"channel"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.svc.OpenSeat(r.Context(), current(r), chi.URLParam(r, "id"), body.PayNow, body.Channel)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) getSeat(w http.ResponseWriter, r *http.Request) {
	row, err := s.svc.DB.GetSeat(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) paySeat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channel string `json:"channel"`
	}
	_ = decode(r, &body)
	row, err := s.svc.PaySeat(r.Context(), current(r), chi.URLParam(r, "id"), body.Channel)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) startBind(w http.ResponseWriter, r *http.Request) {
	row, err := s.svc.StartBind(r.Context(), current(r), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) completeBind(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PriceYuan string `json:"price_yuan"`
		LogonID   string `json:"logon_id"`
	}
	_ = decode(r, &body)
	row, err := s.svc.CompleteBind(r.Context(), current(r), chi.URLParam(r, "id"), body.PriceYuan, body.LogonID)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) modifyCapability(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.svc.ModifyCapability(r.Context(), current(r), chi.URLParam(r, "id"), body.Action)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) simulatePurchase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExternalBuyerID string `json:"external_buyer_id"`
	}
	_ = decode(r, &body)
	row, err := s.svc.SimulatePurchase(r.Context(), current(r), chi.URLParam(r, "id"), body.ExternalBuyerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) getPricing(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListCatalog(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"catalog": rows, "phase1_mode": "ONE_TIME"})
}

func (s *Server) putPricing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		AmountYuan string `json:"amount_yuan"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.svc.UpdateSeatPrice(r.Context(), current(r), body.Name, body.AmountYuan); err != nil {
		writeErr(w, err)
		return
	}
	s.getPricing(w, r)
}

func (s *Server) getCommission(w http.ResponseWriter, r *http.Request) {
	row, err := s.svc.DB.Commission(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) putCommission(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RatePercent      string `json:"rate_percent"`
		JSAPIFeePercent  string `json:"alipay_jsapi_fee_percent"`
		NowPayFeePercent string `json:"alipay_nowpay_fee_percent"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.svc.UpdateCommission(r.Context(), current(r), body.RatePercent, body.JSAPIFeePercent, body.NowPayFeePercent); err != nil {
		writeErr(w, err)
		return
	}
	s.getCommission(w, r)
}

func (s *Server) listPurchases(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListPurchases(r.Context(), r.URL.Query().Get("project_id"), "", limitQ(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"purchases": rows})
}

func (s *Server) listLedger(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListLedger(r.Context(), limitQ(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"entries": rows})
}

func (s *Server) settle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Note string `json:"note"`
	}
	_ = decode(r, &body)
	row, err := s.svc.Settle(r.Context(), current(r), body.Note)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListAudit(r.Context(), limitQ(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"entries": rows})
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListWebhooks(r.Context(), limitQ(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"deliveries": rows})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.svc.ChangeAdminPassword(r.Context(), current(r), body.Current, body.Next); err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) creatorProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListProjects(r.Context(), current(r).CreatorID)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"projects": rows})
}

func (s *Server) creatorProject(w http.ResponseWriter, r *http.Request) {
	s.writeDesk(w, r, chi.URLParam(r, "id"), current(r).CreatorID)
}

func (s *Server) creatorEndpoints(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CallbackURL string `json:"callback_url"`
		NotifyURL   string `json:"notify_url"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.svc.UpdateEndpoints(r.Context(), current(r), chi.URLParam(r, "id"), body.CallbackURL, body.NotifyURL)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) creatorCheckout(w http.ResponseWriter, r *http.Request) {
	row, err := s.svc.OpenSeat(r.Context(), current(r), chi.URLParam(r, "id"), true, "mock_checkout")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) creatorPurchases(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.DB.ListPurchases(r.Context(), "", current(r).CreatorID, limitQ(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"purchases": rows})
}

func (s *Server) mockTicket(w http.ResponseWriter, r *http.Request) {
	view, err := s.svc.LookupTicket(r.Context(), chi.URLParam(r, "ticket"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, view)
}

func (s *Server) mockComplete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PriceYuan string `json:"price_yuan"`
		LogonID   string `json:"logon_id"`
	}
	_ = decode(r, &body)
	row, err := s.svc.CompleteByTicket(r.Context(), chi.URLParam(r, "ticket"), body.PriceYuan, body.LogonID)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) mockPay(w http.ResponseWriter, r *http.Request) {
	row, err := s.svc.PayByTicket(r.Context(), chi.URLParam(r, "ticket"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, row)
}

func (s *Server) writeDesk(w http.ResponseWriter, r *http.Request, projectID, creatorID string) {
	p, err := s.svc.DB.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if creatorID != "" && p.CreatorID != creatorID {
		writeErr(w, domain.E(403, "forbidden", "不能查看其他创作者的项目"))
		return
	}
	seats, err := s.svc.DB.ListSeats(r.Context(), p.ID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	purchases, err := s.svc.DB.ListPurchases(r.Context(), p.ID, "", 50)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok(w, map[string]any{"project": p, "seats": seats, "purchases": purchases})
}

func readProject(r *http.Request) (service.ProjectInput, error) {
	var body struct {
		CreatorID         string  `json:"creator_id"`
		Name              string  `json:"name"`
		Status            string  `json:"status"`
		CallbackURL       string  `json:"callback_url"`
		NotifyURL         string  `json:"notify_url"`
		BillingMode       string  `json:"billing_mode"`
		SeatPriceYuan     *string `json:"seat_price_yuan"`
		CustomerPriceYuan string  `json:"customer_price_yuan"`
		CommissionPercent *string `json:"commission_percent"`
	}
	if err := decode(r, &body); err != nil {
		return service.ProjectInput{}, err
	}
	return service.ProjectInput{
		CreatorID: body.CreatorID, Name: body.Name, Status: body.Status,
		CallbackURL: body.CallbackURL, NotifyURL: body.NotifyURL, BillingMode: body.BillingMode,
		SeatPriceYuan: body.SeatPriceYuan, CustomerPriceYuan: body.CustomerPriceYuan,
		CommissionPercent: body.CommissionPercent,
	}, nil
}

func limitQ(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n
}
