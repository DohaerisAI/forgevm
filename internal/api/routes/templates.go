package routes

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/DohaerisAI/forgevm/internal/httputil"
	"github.com/DohaerisAI/forgevm/internal/orchestrator"
)

type TemplateRoutes struct {
	registry *orchestrator.TemplateRegistry
	manager  *orchestrator.Manager
}

func NewTemplateRoutes(registry *orchestrator.TemplateRegistry, manager *orchestrator.Manager) *TemplateRoutes {
	return &TemplateRoutes{registry: registry, manager: manager}
}

func (t *TemplateRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", t.Create)
	r.Get("/", t.List)
	r.Route("/{name}", func(r chi.Router) {
		r.Get("/", t.Get)
		r.Put("/", t.Update)
		r.Delete("/", t.Delete)
		r.Post("/spawn", t.Spawn)
	})
	return r
}

// Create registers a new template.
//
//	@Summary		Create a template
//	@Description	Register a new sandbox template
//	@Tags			templates
//	@Accept			json
//	@Produce		json
//	@Param			request	body		orchestrator.Template	true	"Template definition"
//	@Success		201		{object}	orchestrator.Template
//	@Failure		400		{object}	httputil.APIError
//	@Failure		409		{object}	httputil.APIError
//	@Failure		500		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/templates [post]
func (t *TemplateRoutes) Create(w http.ResponseWriter, r *http.Request) {
	var tmpl orchestrator.Template
	if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	if err := t.registry.Create(r.Context(), &tmpl); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			httputil.WriteError(w, http.StatusConflict, httputil.CodeConflict, "template already exists")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, tmpl)
}

// List returns all templates.
//
//	@Summary		List templates
//	@Description	Return all registered templates
//	@Tags			templates
//	@Produce		json
//	@Success		200	{array}		orchestrator.Template
//	@Failure		500	{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/templates [get]
func (t *TemplateRoutes) List(w http.ResponseWriter, r *http.Request) {
	templates, err := t.registry.List(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if templates == nil {
		templates = []*orchestrator.Template{}
	}
	httputil.WriteJSON(w, http.StatusOK, templates)
}

// Get returns a template by name.
//
//	@Summary		Get a template
//	@Description	Return a template by its name
//	@Tags			templates
//	@Produce		json
//	@Param			name	path		string	true	"Template name"
//	@Success		200		{object}	orchestrator.Template
//	@Failure		404		{object}	httputil.APIError
//	@Failure		500		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/templates/{name} [get]
func (t *TemplateRoutes) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	tmpl, err := t.registry.Get(r.Context(), name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, tmpl)
}

// Update updates an existing template.
//
//	@Summary		Update a template
//	@Description	Update an existing template by name
//	@Tags			templates
//	@Accept			json
//	@Produce		json
//	@Param			name	path		string				true	"Template name"
//	@Param			request	body		orchestrator.Template	true	"Updated template"
//	@Success		200		{object}	orchestrator.Template
//	@Failure		400		{object}	httputil.APIError
//	@Failure		404		{object}	httputil.APIError
//	@Failure		500		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/templates/{name} [put]
func (t *TemplateRoutes) Update(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var tmpl orchestrator.Template
	if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	tmpl.Name = name
	if err := t.registry.Update(r.Context(), &tmpl); err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, tmpl)
}

// Delete removes a template.
//
//	@Summary		Delete a template
//	@Description	Delete a template by name
//	@Tags			templates
//	@Produce		json
//	@Param			name	path		string	true	"Template name"
//	@Success		200		{object}	StatusResponse
//	@Failure		404		{object}	httputil.APIError
//	@Failure		500		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/templates/{name} [delete]
func (t *TemplateRoutes) Delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := t.registry.Delete(r.Context(), name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Spawn creates a sandbox from a template.
//
//	@Summary		Spawn from template
//	@Description	Create a new sandbox using a template's configuration, with optional overrides
//	@Tags			templates
//	@Accept			json
//	@Produce		json
//	@Param			name	path		string					true	"Template name"
//	@Param			request	body		TemplateSpawnOverrides	false	"Optional overrides"
//	@Success		201		{object}	orchestrator.Sandbox
//	@Failure		404		{object}	httputil.APIError
//	@Failure		500		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/templates/{name}/spawn [post]
func (t *TemplateRoutes) Spawn(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	tmpl, err := t.registry.Get(r.Context(), name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	// Allow overrides from request body
	var overrides struct {
		Provider string `json:"provider,omitempty"`
		TTL      string `json:"ttl,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&overrides)

	req := t.registry.ToSpawnRequest(tmpl)
	if overrides.Provider != "" {
		req.Provider = overrides.Provider
	}
	if overrides.TTL != "" {
		req.TTL = overrides.TTL
	}

	sb, err := t.manager.Spawn(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, sb)
}
