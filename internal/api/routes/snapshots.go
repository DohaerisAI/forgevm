package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/DohaerisAI/forgevm/internal/httputil"
	"github.com/DohaerisAI/forgevm/internal/providers"
)

type SnapshotRoutes struct {
	registry *providers.Registry
}

func NewSnapshotRoutes(registry *providers.Registry) *SnapshotRoutes {
	return &SnapshotRoutes{registry: registry}
}

func (s *SnapshotRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", s.List)
	return r
}

// List returns all available snapshots across providers.
//
//	@Summary		List snapshots
//	@Description	Return all pre-built VM snapshots available for fast restore
//	@Tags			snapshots
//	@Produce		json
//	@Success		200	{array}		providers.SnapshotSummary
//	@Security		ApiKeyAuth
//	@Router			/snapshots [get]
func (s *SnapshotRoutes) List(w http.ResponseWriter, r *http.Request) {
	var all []providers.SnapshotSummary

	for _, name := range s.registry.List() {
		prov, err := s.registry.Get(name)
		if err != nil {
			continue
		}
		if lister, ok := prov.(providers.SnapshotLister); ok {
			if snaps := lister.ListSnapshots(); snaps != nil {
				all = append(all, snaps...)
			}
		}
	}

	if all == nil {
		all = []providers.SnapshotSummary{}
	}

	httputil.WriteJSON(w, http.StatusOK, all)
}
