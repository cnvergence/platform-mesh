package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	appcontext "github.com/platform-mesh/search/internal/context"
	"github.com/platform-mesh/search/internal/service/search"
)

type SearchService interface {
	Search(ctx context.Context, req search.SearchRequest) (search.SearchResponse, error)
}

func CreateRouter(svc SearchService, mws []func(http.Handler) http.Handler) *chi.Mux {
	router := chi.NewRouter()

	router.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	router.With(mws...).Get("/rest/v1/search", func(w http.ResponseWriter, r *http.Request) {
		rc, err := appcontext.GetRequestContext(r.Context())
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		limit := 0
		limitRaw := strings.TrimSpace(r.URL.Query().Get("limit"))
		if limitRaw != "" {
			parsed, err := strconv.Atoi(limitRaw)
			if err != nil {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			limit = parsed
		}

		resp, err := svc.Search(r.Context(), search.SearchRequest{
			Organization: rc.Organization,
			User:         rc.User,
			Query:        q,
			Limit:        limit,
			Cursor:       strings.TrimSpace(r.URL.Query().Get("cursor")),
		})
		if err != nil {
			switch {
			case errors.Is(err, search.ErrInvalidRequest), errors.Is(err, search.ErrInvalidCursor):
				http.Error(w, err.Error(), http.StatusBadRequest)
			case errors.Is(err, search.ErrUnauthorized):
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			case errors.Is(err, search.ErrForbidden):
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			default:
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})

	return router
}
