package routes

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/DohaerisAI/forgevm/internal/httputil"
	"github.com/DohaerisAI/forgevm/internal/providers"
)

type sandboxCounter interface {
	CountByProvider(ctx context.Context, provider string) int
}

type ProviderRoutes struct {
	registry *providers.Registry
	counter  sandboxCounter
}

func NewProviderRoutes(registry *providers.Registry, counter sandboxCounter) *ProviderRoutes {
	return &ProviderRoutes{registry: registry, counter: counter}
}

func (p *ProviderRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", p.List)
	r.Post("/test", p.Test)
	r.Get("/{providerName}", p.Detail)
	return r
}

// ProviderInfo is the summary info for a provider.
type ProviderInfo struct {
	Name    string `json:"name" example:"firecracker"`
	Healthy bool   `json:"healthy" example:"true"`
	Default bool   `json:"default" example:"true"`
}

// ProviderDetail is the detailed info for a provider.
type ProviderDetail struct {
	Name         string            `json:"name" example:"firecracker"`
	Healthy      bool              `json:"healthy" example:"true"`
	Default      bool              `json:"default" example:"true"`
	SandboxCount int               `json:"sandbox_count" example:"3"`
	Config       map[string]string `json:"config"`
}

// List returns all registered providers.
//
//	@Summary		List providers
//	@Description	Return all registered providers with health status
//	@Tags			providers
//	@Produce		json
//	@Success		200	{array}		ProviderInfo
//	@Security		ApiKeyAuth
//	@Router			/providers [get]
func (p *ProviderRoutes) List(w http.ResponseWriter, r *http.Request) {
	names := p.registry.List()
	infos := make([]ProviderInfo, 0, len(names))
	dflt := p.registry.Default()

	for _, name := range names {
		prov, err := p.registry.Get(name)
		if err != nil {
			continue
		}
		infos = append(infos, ProviderInfo{
			Name:    name,
			Healthy: prov.Healthy(r.Context()),
			Default: name == dflt,
		})
	}

	httputil.WriteJSON(w, http.StatusOK, infos)
}

// Test checks health of all providers.
//
//	@Summary		Test providers
//	@Description	Run health checks on all registered providers
//	@Tags			providers
//	@Produce		json
//	@Success		200	{object}	map[string]bool
//	@Security		ApiKeyAuth
//	@Router			/providers/test [post]
func (p *ProviderRoutes) Test(w http.ResponseWriter, r *http.Request) {
	names := p.registry.List()
	results := make(map[string]bool, len(names))
	ctx := context.Background()

	for _, name := range names {
		prov, err := p.registry.Get(name)
		if err != nil {
			results[name] = false
			continue
		}
		results[name] = prov.Healthy(ctx)
	}

	httputil.WriteJSON(w, http.StatusOK, results)
}

// Detail returns detailed information about a provider.
//
//	@Summary		Get provider details
//	@Description	Return detailed information about a specific provider
//	@Tags			providers
//	@Produce		json
//	@Param			providerName	path		string	true	"Provider name"
//	@Success		200				{object}	ProviderDetail
//	@Failure		404				{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/providers/{providerName} [get]
func (p *ProviderRoutes) Detail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "providerName")

	prov, err := p.registry.Get(name)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "provider not found")
		return
	}

	dflt := p.registry.Default()
	cfg := make(map[string]string)

	// Expose safe config info based on provider type
	switch v := prov.(type) {
	case *providers.FirecrackerProvider:
		c := v.ProviderConfig()
		cfg["firecracker_path"] = c.FirecrackerPath
		cfg["kernel_path"] = c.KernelPath
		cfg["default_rootfs"] = c.DefaultRootfs
		cfg["data_dir"] = c.DataDir
		cfg["type"] = "firecracker"
	case *providers.MockProvider:
		cfg["type"] = "mock"
	case *providers.PRootProvider:
		c := v.ProviderConfig()
		cfg["rootfs_path"] = c.RootfsPath
		cfg["proot_binary"] = c.PRootBinary
		cfg["workspace_base"] = c.WorkspaceBase
		cfg["type"] = "proot"
	default:
		cfg["type"] = "other"
	}

	count := 0
	if p.counter != nil {
		count = p.counter.CountByProvider(r.Context(), name)
	}

	httputil.WriteJSON(w, http.StatusOK, ProviderDetail{
		Name:         name,
		Healthy:      prov.Healthy(r.Context()),
		Default:      name == dflt,
		SandboxCount: count,
		Config:       cfg,
	})
}
