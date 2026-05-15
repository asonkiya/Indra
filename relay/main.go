// relay is a lightweight push notification relay for Indra.
//
// It stores zero message content — only maps peerID → push token. When a sender
// cannot reach a recipient directly (and has stored the message in the DHT
// mailbox), it POSTs to this relay, which sends a silent push to wake the
// recipient's device. The device then polls the DHT mailbox to retrieve the
// encrypted message.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	p8Path := flag.String("apns-p8", "", "path to APNs .p8 key file")
	keyID := flag.String("apns-key-id", "", "APNs key ID")
	teamID := flag.String("apns-team-id", "", "Apple team ID")
	topic := flag.String("apns-topic", "", "APNs topic (bundle ID)")
	prod := flag.Bool("apns-prod", false, "use production APNs endpoint")
	flag.Parse()

	// Override flags with env vars if set.
	if v := os.Getenv("APNS_P8_PATH"); v != "" && *p8Path == "" {
		*p8Path = v
	}
	if v := os.Getenv("APNS_KEY_ID"); v != "" && *keyID == "" {
		*keyID = v
	}
	if v := os.Getenv("APNS_TEAM_ID"); v != "" && *teamID == "" {
		*teamID = v
	}
	if v := os.Getenv("APNS_TOPIC"); v != "" && *topic == "" {
		*topic = v
	}
	if os.Getenv("APNS_PROD") == "true" && !*prod {
		*prod = true
	}

	var apns *APNsPusher
	if *p8Path != "" {
		var err error
		apns, err = NewAPNsPusher(*p8Path, *keyID, *teamID, *topic, *prod)
		if err != nil {
			log.Fatalf("failed to init APNs: %v", err)
		}
		log.Printf("APNs configured: key=%s team=%s topic=%s prod=%v", *keyID, *teamID, *topic, *prod)
	} else {
		log.Println("WARNING: APNs not configured — push notifications will fail")
	}

	store := NewStore()
	pusher := NewPusher(apns)
	srv := &server{store: store, pusher: pusher}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", srv.handleRegister)
	mux.HandleFunc("POST /notify", srv.handleNotify)
	mux.HandleFunc("GET /health", srv.handleHealth)

	log.Printf("relay listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

type server struct {
	store  *Store
	pusher *Pusher
}

type registerRequest struct {
	PeerID   string `json:"peer_id"`
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

type notifyRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
		return
	}

	if req.PeerID == "" || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "peer_id and token are required"})
		return
	}

	platform := strings.ToLower(req.Platform)
	if platform == "" {
		platform = "ios"
	}
	if platform != "ios" && platform != "android" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "platform must be 'ios' or 'android'"})
		return
	}

	s.store.Register(req.PeerID, Registration{Token: req.Token, Platform: platform})
	log.Printf("registered %s (%s)", req.PeerID[:16]+"...", platform)
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleNotify(w http.ResponseWriter, r *http.Request) {
	var req notifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
		return
	}

	if req.To == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "'to' is required"})
		return
	}

	reg, ok := s.store.Lookup(req.To)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "peer not registered"})
		return
	}

	if err := s.pusher.Notify(reg); err != nil {
		log.Printf("push failed for %s: %v", req.To[:16]+"...", err)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: fmt.Sprintf("push failed: %v", err)})
		return
	}

	log.Printf("notified %s (from %s)", req.To[:16]+"...", req.From[:16]+"...")
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"registered":  s.store.Count(),
		"apns_active": s.pusher.apns != nil,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
