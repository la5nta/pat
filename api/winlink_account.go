package api

import (
	"encoding/json"
	"net/http"

	"github.com/la5nta/pat/internal/cmsapi"
)

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
