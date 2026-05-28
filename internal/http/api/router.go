package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mstilde/unipile-linkedin-go/internal/auth"
	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

// Deps groups the wiring an API router needs. All fields are required.
type Deps struct {
	Pool   *pgxpool.Pool
	Q      *gen.Queries
	Signer *auth.Signer
	Store  *SQLAccountStore
}

// Mount returns a chi router with the v1 API routes wired up.
// Public:   POST /api/v1/login
// Auth:     GET  /api/v1/me
// Per-account scope (requires ownership of {accountID}):
//
//	GET    /api/v1/accounts/{accountID}/campaigns
//	POST   /api/v1/accounts/{accountID}/campaigns
//	GET    /api/v1/accounts/{accountID}/campaigns/{campaignID}
//	PATCH  /api/v1/accounts/{accountID}/campaigns/{campaignID}
//	DELETE /api/v1/accounts/{accountID}/campaigns/{campaignID}
//	POST   /api/v1/accounts/{accountID}/campaigns/{campaignID}/start|pause|resume|duplicate
//	GET    /api/v1/accounts/{accountID}/campaigns/{campaignID}/funnel
//	GET    /api/v1/accounts/{accountID}/campaigns/{campaignID}/sequence
//	PUT    /api/v1/accounts/{accountID}/campaigns/{campaignID}/sequence
//	GET    /api/v1/accounts/{accountID}/campaigns/{campaignID}/templates
//	PUT    /api/v1/accounts/{accountID}/campaigns/{campaignID}/templates
//	DELETE /api/v1/accounts/{accountID}/campaigns/{campaignID}/templates/{kind}
//	GET    /api/v1/accounts/{accountID}/campaigns/{campaignID}/prospects
//	POST   /api/v1/accounts/{accountID}/campaigns/{campaignID}/prospects (multipart CSV)
//	GET    /api/v1/accounts/{accountID}/campaigns/{campaignID}/prospects/{prospectID}
//	DELETE /api/v1/accounts/{accountID}/campaigns/{campaignID}/prospects/{prospectID}
func Mount(d Deps) chi.Router {
	authH := &AuthHandler{Q: d.Q, Signer: d.Signer}
	campH := &CampaignsHandler{Q: d.Q}
	seqH := &SequenceHandler{Pool: d.Pool, Q: d.Q}
	prosH := &ProspectsHandler{Pool: d.Pool, Q: d.Q}

	r := chi.NewRouter()

	r.Post("/login", authH.Login)

	r.Group(func(r chi.Router) {
		r.Use(auth.Authenticate(d.Signer))
		r.Get("/me", authH.Me)

		r.Route("/accounts/{accountID}", func(r chi.Router) {
			r.Use(auth.RequireOwnedAccount(d.Store))

			r.Route("/campaigns", func(r chi.Router) {
				r.Get("/", campH.List)
				r.Post("/", campH.Create)

				r.Route("/{campaignID}", func(r chi.Router) {
					r.Get("/", campH.Get)
					r.Patch("/", campH.Update)
					r.Delete("/", campH.Delete)
					r.Post("/start", campH.Start)
					r.Post("/pause", campH.Pause)
					r.Post("/resume", campH.Resume)
					r.Post("/duplicate", campH.Duplicate)
					r.Get("/funnel", campH.Funnel)

					r.Get("/sequence", seqH.List)
					r.Put("/sequence", seqH.Replace)

					r.Get("/templates", seqH.ListTemplates)
					r.Put("/templates", seqH.UpsertTemplate)
					r.Delete("/templates/{kind}", seqH.DeleteTemplate)

					r.Get("/prospects", prosH.List)
					r.Post("/prospects", prosH.Upload)
					r.Get("/prospects/{prospectID}", prosH.Get)
					r.Delete("/prospects/{prospectID}", prosH.Delete)
				})
			})
		})
	})

	return r
}
