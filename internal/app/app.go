package app

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/vsaluzzo/minecraft-home-portal/internal/config"
	"github.com/vsaluzzo/minecraft-home-portal/internal/discovery"
	"github.com/vsaluzzo/minecraft-home-portal/internal/minecraft"
	"github.com/vsaluzzo/minecraft-home-portal/internal/store"
)

//go:embed web/templates/*.gohtml web/static/*
var assets embed.FS

type App struct {
	cfg       config.Config
	store     *store.Store
	discovery *discovery.Service
	minecraft *minecraft.Client
	templates *template.Template
	static    http.Handler
}

type TemplateData struct {
	CurrentUser *store.User
	Servers     []discovery.Server
	Server      discovery.Server
	Error       string
	Flash       string
}

func New(cfg config.Config) (*App, error) {
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	if err := db.EnsureBootstrapAdmin(context.Background(), cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		db.Close()
		return nil, err
	}

	rcon := minecraft.NewClient()

	dockerClient, err := discovery.NewDockerClient(cfg.DockerHost)
	if err != nil {
		log.Printf("docker client initialization warning: %v", err)
	}

	discoveryService := discovery.New(cfg.LabelNamespace, dockerClient, rcon)
	if err := discoveryService.Refresh(context.Background()); err != nil {
		log.Printf("initial discovery warning: %v", err)
	}

	funcs := template.FuncMap{
		"join": strings.Join,
	}

	tmpl, err := template.New("").Funcs(funcs).ParseFS(assets, "web/templates/*.gohtml")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	app := &App{
		cfg:       cfg,
		store:     db,
		discovery: discoveryService,
		minecraft: rcon,
		templates: tmpl,
		static:    http.FileServer(http.FS(assets)),
	}

	go app.discoveryLoop()

	return app, nil
}

func (a *App) Close() error {
	if a.discovery != nil {
		if err := a.discovery.Close(); err != nil {
			log.Printf("docker client close warning: %v", err)
		}
	}
	if a.store != nil {
		return a.store.Close()
	}
	return nil
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /healthz", http.HandlerFunc(a.handleHealth))
	mux.Handle("GET /web/static/", http.StripPrefix("/", a.static))
	mux.Handle("GET /login", http.HandlerFunc(a.handleLoginPage))
	mux.Handle("POST /login", http.HandlerFunc(a.handleLogin))
	mux.Handle("POST /logout", a.requireAuth(http.HandlerFunc(a.handleLogout)))
	mux.Handle("GET /", http.HandlerFunc(a.handleDashboard))
	mux.Handle("GET /servers/{id}", http.HandlerFunc(a.handleServerDetail))
	mux.Handle("POST /servers/{id}/start", a.requireAdmin(http.HandlerFunc(a.handleServerStart)))
	mux.Handle("POST /servers/{id}/stop", a.requireAdmin(http.HandlerFunc(a.handleServerStop)))
	mux.Handle("POST /servers/{id}/restart", a.requireAdmin(http.HandlerFunc(a.handleServerRestart)))
	mux.Handle("POST /servers/{id}/rcon/op", a.requireAdmin(http.HandlerFunc(a.handleRCONOp)))
	mux.Handle("POST /servers/{id}/rcon/deop", a.requireAdmin(http.HandlerFunc(a.handleRCONDeop)))
	mux.Handle("POST /servers/{id}/rcon/say", a.requireAdmin(http.HandlerFunc(a.handleRCONSay)))

	return a.withCurrentUser(mux)
}

func (a *App) discoveryLoop() {
	ticker := time.NewTicker(a.cfg.DiscoveryRefresh)
	defer ticker.Stop()

	for range ticker.C {
		if err := a.discovery.Refresh(context.Background()); err != nil {
			log.Printf("discovery refresh warning: %v", err)
		}
	}
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUserFromContext(r.Context())
	servers := a.visibleServers(user)

	a.render(w, http.StatusOK, "dashboard", TemplateData{
		CurrentUser: user,
		Servers:     servers,
		Flash:       r.URL.Query().Get("flash"),
	})
}

func (a *App) handleServerDetail(w http.ResponseWriter, r *http.Request) {
	user := currentUserFromContext(r.Context())
	server, ok := a.discovery.ByID(r.PathValue("id"))
	if !ok || !a.canViewServer(user, server) {
		http.NotFound(w, r)
		return
	}

	a.render(w, http.StatusOK, "server", TemplateData{
		CurrentUser: user,
		Server:      server,
	})
}

func (a *App) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if currentUserFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	a.render(w, http.StatusOK, "login", TemplateData{
		Error: r.URL.Query().Get("error"),
	})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.render(w, http.StatusBadRequest, "login", TemplateData{Error: "Invalid form"})
		return
	}

	user, err := a.store.AuthenticateUser(r.Context(), strings.TrimSpace(r.FormValue("username")), r.FormValue("password"))
	if err != nil {
		a.render(w, http.StatusUnauthorized, "login", TemplateData{Error: "Invalid credentials"})
		return
	}

	session, err := a.store.CreateSession(r.Context(), user.ID, a.cfg.SessionTTL)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     a.cfg.SessionCookieName,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(a.cfg.SessionCookieName); err == nil {
		_ = a.store.DeleteSession(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     a.cfg.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/?flash=Signed+out", http.StatusSeeOther)
}

func (a *App) handleServerStart(w http.ResponseWriter, r *http.Request) {
	a.runAdminAction(w, r, "server.start", func(server discovery.Server) error {
		if !server.Actions.Start {
			return fmt.Errorf("start is disabled for this server")
		}
		return a.discovery.Start(r.Context(), server.ID)
	})
}

func (a *App) handleServerStop(w http.ResponseWriter, r *http.Request) {
	a.runAdminAction(w, r, "server.stop", func(server discovery.Server) error {
		if !server.Actions.Stop {
			return fmt.Errorf("stop is disabled for this server")
		}
		return a.discovery.Stop(r.Context(), server.ID)
	})
}

func (a *App) handleServerRestart(w http.ResponseWriter, r *http.Request) {
	a.runAdminAction(w, r, "server.restart", func(server discovery.Server) error {
		if !server.Actions.Restart {
			return fmt.Errorf("restart is disabled for this server")
		}
		return a.discovery.Restart(r.Context(), server.ID)
	})
}

func (a *App) handleRCONOp(w http.ResponseWriter, r *http.Request) {
	a.runRCONCommand(w, r, "server.rcon.op", func(server discovery.Server) bool {
		return server.Actions.Op
	}, func(server discovery.Server, player string) string {
		return "op " + player
	})
}

func (a *App) handleRCONDeop(w http.ResponseWriter, r *http.Request) {
	a.runRCONCommand(w, r, "server.rcon.deop", func(server discovery.Server) bool {
		return server.Actions.Deop
	}, func(server discovery.Server, player string) string {
		return "deop " + player
	})
}

func (a *App) handleRCONSay(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		http.Redirect(w, r, "/servers/"+r.PathValue("id"), http.StatusSeeOther)
		return
	}

	a.runAdminAction(w, r, "server.rcon.say", func(server discovery.Server) error {
		if !server.Actions.Say {
			return fmt.Errorf("say is disabled for this server")
		}

		_, err := a.minecraft.Execute(r.Context(), server, "say "+message)
		return err
	})
}

func (a *App) runRCONCommand(
	w http.ResponseWriter,
	r *http.Request,
	action string,
	enabled func(server discovery.Server) bool,
	commandBuilder func(server discovery.Server, player string) string,
) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	player := strings.TrimSpace(r.FormValue("player"))
	if player == "" {
		http.Redirect(w, r, "/servers/"+r.PathValue("id"), http.StatusSeeOther)
		return
	}

	a.runAdminAction(w, r, action, func(server discovery.Server) error {
		if !enabled(server) {
			return fmt.Errorf("action is disabled for this server")
		}
		command := commandBuilder(server, player)
		_, err := a.minecraft.Execute(r.Context(), server, command)
		return err
	})
}

func (a *App) runAdminAction(w http.ResponseWriter, r *http.Request, action string, fn func(server discovery.Server) error) {
	user := currentUserFromContext(r.Context())
	server, ok := a.discovery.ByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	err := fn(server)
	metadata, _ := json.Marshal(map[string]string{
		"server_name": server.Name,
		"container":   server.ContainerName,
	})
	actorID := user.ID
	_ = a.store.WriteAuditLog(r.Context(), &actorID, action, "server", server.ID, string(metadata))

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if refreshErr := a.discovery.Refresh(r.Context()); refreshErr != nil {
		log.Printf("post-action refresh warning: %v", refreshErr)
	}

	http.Redirect(w, r, "/servers/"+server.ID, http.StatusSeeOther)
}

func (a *App) visibleServers(user *store.User) []discovery.Server {
	servers := a.discovery.All()
	if user != nil {
		return servers
	}

	filtered := make([]discovery.Server, 0, len(servers))
	for _, server := range servers {
		if server.Visibility == discovery.VisibilityPublic {
			filtered = append(filtered, server)
		}
	}
	return filtered
}

func (a *App) canViewServer(user *store.User, server discovery.Server) bool {
	if user != nil {
		return true
	}
	return server.Visibility == discovery.VisibilityPublic
}

func (a *App) render(w http.ResponseWriter, status int, name string, data TemplateData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := a.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
