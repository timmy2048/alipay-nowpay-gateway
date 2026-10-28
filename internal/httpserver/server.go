package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yin/alipay-nowpay-gateway/internal/docs"
	"github.com/yin/alipay-nowpay-gateway/internal/domain"
	"github.com/yin/alipay-nowpay-gateway/internal/service"
)

type Server struct {
	svc     *service.Service
	mode    string
	origins []string
	secure  bool
	ttl     time.Duration
}

func New(svc *service.Service, alipayMode string, origins []string, secure bool, ttl time.Duration) *Server {
	return &Server{svc: svc, mode: alipayMode, origins: origins, secure: secure, ttl: ttl}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, s.cors, s.guardOrigin)
	r.Get("/api/health", s.health)
	r.Get("/api/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(docs.OpenAPI)
	})
	r.Post("/api/auth/login", s.login)
	r.Post("/api/auth/logout", s.logout)
	r.Get("/api/mock/alipay/tickets/{ticket}", s.mockTicket)
	r.Post("/api/mock/alipay/tickets/{ticket}/complete", s.mockComplete)
	r.Post("/api/mock/alipay/tickets/{ticket}/pay", s.mockPay)
	r.Group(func(r chi.Router) {
		r.Use(s.authenticated)
		r.Get("/api/auth/me", s.me)
		r.Route("/api/admin", func(r chi.Router) {
			r.Use(s.adminOnly)
			r.Get("/dashboard", s.dashboard)
			r.Get("/creators", s.listCreators)
			r.Post("/creators", s.createCreator)
			r.Get("/creators/{id}", s.getCreator)
			r.Put("/creators/{id}", s.updateCreator)
			r.Post("/creators/{id}/password", s.resetCreatorPassword)
			r.Get("/projects", s.listProjects)
			r.Post("/projects", s.createProject)
			r.Get("/projects/{id}", s.getProject)
			r.Put("/projects/{id}", s.updateProject)
			r.Post("/projects/{id}/seats", s.createSeat)
			r.Get("/seats/{id}", s.getSeat)
			r.Post("/seats/{id}/pay", s.paySeat)
			r.Post("/seats/{id}/bind", s.startBind)
			r.Post("/seats/{id}/bind/complete", s.completeBind)
			r.Post("/seats/{id}/capability", s.modifyCapability)
			r.Post("/seats/{id}/purchases", s.simulatePurchase)
			r.Get("/pricing", s.getPricing)
			r.Put("/pricing", s.putPricing)
			r.Get("/commission", s.getCommission)
			r.Put("/commission", s.putCommission)
			r.Get("/purchases", s.listPurchases)
			r.Get("/ledger", s.listLedger)
			r.Post("/settlements", s.settle)
			r.Get("/audit", s.listAudit)
			r.Get("/webhooks", s.listWebhooks)
			r.Put("/password", s.changePassword)
		})
		r.Route("/api/creator", func(r chi.Router) {
			r.Use(s.creatorOnly)
			r.Get("/projects", s.creatorProjects)
			r.Get("/projects/{id}", s.creatorProject)
			r.Put("/projects/{id}/endpoints", s.creatorEndpoints)
			r.Post("/projects/{id}/seats", s.creatorCheckout)
			r.Post("/seats/{id}/bind", s.startBind)
			r.Post("/seats/{id}/bind/complete", s.completeBind)
			r.Get("/purchases", s.creatorPurchases)
		})
	})
	return r
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && s.originAllowed(origin, r.Host) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) guardOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && !s.originAllowed(origin, r.Host) {
			writeErr(w, domain.E(403, "bad_origin", "请求来源不被接受"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(origin, host string) bool {
	if origin == "null" || origin == "" {
		return false
	}
	for _, allowed := range s.origins {
		if origin == allowed {
			return true
		}
	}
	return origin == "http://"+host || origin == "https://"+host
}

func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, err := s.svc.Session(r.Context(), tokenFrom(r))
		if err != nil {
			writeErr(w, err)
			return
		}
		actor.IP = clientIP(r)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actor)))
	})
}

func (s *Server) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if current(r).Type != "admin" {
			writeErr(w, domain.E(403, "forbidden", "需要管理员登录"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) creatorOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if current(r).Type != "creator" {
			writeErr(w, domain.E(403, "forbidden", "需要创作者登录"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type actorKey struct{}

func current(r *http.Request) service.Actor {
	a, _ := r.Context().Value(actorKey{}).(service.Actor)
	return a
}

func tokenFrom(r *http.Request) string {
	if c, err := r.Cookie("yin_session"); err == nil && c.Value != "" {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func (s *Server) setSession(w http.ResponseWriter, raw string) {
	http.SetCookie(w, &http.Cookie{
		Name: "yin_session", Value: raw, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: s.secure, MaxAge: int(s.ttl.Seconds()),
	})
}

func (s *Server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: "yin_session", Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: s.secure, MaxAge: -1,
	})
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return domain.E(400, "bad_json", "请求体不是有效的 JSON")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode", "err", err)
	}
}

func writeErr(w http.ResponseWriter, err error) {
	pub := service.Public(err)
	de := domain.As(pub)
	if de == nil {
		de = domain.E(500, "internal", "系统繁忙，请稍后再试")
	}
	if de.Status == 0 {
		de.Status = 400
	}
	writeJSON(w, de.Status, map[string]any{"error": map[string]string{"code": de.Code, "message": de.Message}})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ok(w http.ResponseWriter, v any) { writeJSON(w, http.StatusOK, v) }
