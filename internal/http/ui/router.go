package ui

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mstilde/unipile-linkedin-go/internal/auth"
	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Q      *gen.Queries
	Signer *auth.Signer
}

// Mount returns the chi router for the server-rendered HTML routes.
// Public:  GET /login, POST /login, POST /logout
// Auth:    GET /dashboard, GET/POST /accounts/{accountID}/campaigns, etc.
func Mount(d Deps) chi.Router {
	h := &Handlers{Pool: d.Pool, Q: d.Q, Signer: d.Signer}
	r := chi.NewRouter()

	r.Get("/login", h.LoginGet)
	r.Post("/login", h.LoginPost)
	r.Post("/logout", h.Logout)

	r.Group(func(r chi.Router) {
		r.Use(requireSession(d.Signer))

		r.Get("/dashboard", h.Dashboard)
		r.Get("/accounts/{accountID}/campaigns", h.CampaignsPage)
		r.Get("/accounts/{accountID}/campaigns/{campaignID}/sequence", h.SequencePage)

		// HTMX endpoints under /ui to keep them distinct from full-page routes.
		r.Route("/ui/accounts/{accountID}/campaigns", func(r chi.Router) {
			r.Post("/", h.CreateCampaign)
			r.Post("/{campaignID}/start", h.StartCampaign)
			r.Post("/{campaignID}/pause", h.PauseCampaign)
			// PUT /sequence is handled by the JSON API; the form-encoded version
			// here would be redundant. The JS converts to JSON before sending.
		})
	})

	return r
}
