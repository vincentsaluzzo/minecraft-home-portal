package app

import (
	"context"
	"net/http"

	"github.com/vsaluzzo/minecraft-home-portal/internal/store"
)

type contextKey string

const userContextKey contextKey = "current_user"

func (a *App) withCurrentUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(a.cfg.SessionCookieName)
		if err == nil && cookie.Value != "" {
			if user, lookupErr := a.store.UserBySessionToken(r.Context(), cookie.Value); lookupErr == nil {
				ctx := context.WithValue(r.Context(), userContextKey, &user)
				r = r.WithContext(ctx)
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUserFromContext(r.Context()) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := currentUserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if user.Role != store.RoleAdmin {
			http.Error(w, "Admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentUserFromContext(ctx context.Context) *store.User {
	user, _ := ctx.Value(userContextKey).(*store.User)
	return user
}
