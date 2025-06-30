package api

import (
	"encoding/json"
	"net/http"

	"github.com/la5nta/pat/internal/cmsapi"
)

func (h Handler) winlinkAccountRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		callsign := h.Options().MyCall
		if v := r.URL.Query().Get("callsign"); v != "" {
			callsign = v
		}
		if callsign == "" {
			http.Error(w, "Empty callsign", http.StatusBadRequest)
			return
		}
		exists, err := cmsapi.AccountExists(r.Context(), callsign)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(struct {
			Callsign string `json:"callsign"`
			Exists   bool   `json:"exists"`
		}{callsign, exists})
	case http.MethodPost:
		type body struct {
			Callsign      string `json:"callsign"`
			Password      string `json:"password"`
			RecoveryEmail string `json:"recovery_email"` // optional
		}
		var v body
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch {
		case v.Callsign == "":
			http.Error(w, "Empty callsign", http.StatusBadRequest)
			return
		case len(v.Password) < 6 || len(v.Password) > 12:
			http.Error(w, "Password must be 6-12 characters", http.StatusBadRequest)
			return
		}
		if err := cmsapi.AccountAdd(r.Context(), v.Callsign, v.Password, v.RecoveryEmail); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(v)
	}
}

func (h Handler) winlinkPasswordRecoveryEmailHandler(w http.ResponseWriter, r *http.Request) {
	type body struct {
		RecoveryEmail string `json:"recovery_email"`
	}

	var (
		ctx      = r.Context()
		callsign = h.Options().MyCall
		password = h.Config().SecureLoginPassword
	)
	if callsign == "" || password == "" {
		http.Error(w, "Missing callsign or password in config", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		email, err := cmsapi.PasswordRecoveryEmailGet(ctx, callsign, password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body{RecoveryEmail: email})
	case http.MethodPut:
		var v body
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := cmsapi.PasswordRecoveryEmailSet(ctx, callsign, password, v.RecoveryEmail); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
}
