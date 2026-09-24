// fakestore stands in for hive-store's /api/me and /api/tokens in the auth
// end-to-end test. It enforces what the real store's verifier enforces that the
// CLI can get wrong: a Bearer that the issuer accepts (asked of its userinfo
// endpoint), typ=Bearer, and aud containing hive-store.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"sync"
)

type claims struct {
	Typ   string `json:"typ"`
	Aud   any    `json:"aud"`
	Azp   string `json:"azp"`
	Email string `json:"email"`
}

func (c claims) hasAud(want string) bool {
	switch a := c.Aud.(type) {
	case string:
		return a == want
	case []any:
		for _, v := range a {
			if v == want {
				return true
			}
		}
	}
	return false
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "listen address")
	userinfo := flag.String("userinfo", "", "issuer userinfo endpoint")
	flag.Parse()

	var mu sync.Mutex
	live := map[string]bool{}

	verify := func(w http.ResponseWriter, r *http.Request) (*claims, bool) {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		parts := strings.Split(bearer, ".")
		if len(parts) != 3 {
			http.Error(w, `{"error":"unauthenticated"}`, 401)
			return nil, false
		}
		raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var c claims
		json.Unmarshal(raw, &c)
		if c.Typ != "Bearer" || !c.hasAud("hive-store") {
			log.Printf("reject: typ=%q aud=%v azp=%q", c.Typ, c.Aud, c.Azp)
			http.Error(w, `{"error":"unauthenticated"}`, 401)
			return nil, false
		}
		req, _ := http.NewRequest("GET", *userinfo, nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			log.Printf("reject: userinfo %v", err)
			http.Error(w, `{"error":"unauthenticated"}`, 401)
			return nil, false
		}
		resp.Body.Close()
		return &c, true
	}

	http.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		c, ok := verify(w, r)
		if !ok {
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"email": c.Email, "operator": false, "balances": map[string]any{},
			"subscription": map[string]any{"plan": "pro", "status": "active", "provider": "card",
				"entitles": []string{"io.github.hive-agi:hive-carto"}},
		})
	})
	http.HandleFunc("/api/tokens", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := verify(w, r); !ok || r.Method != http.MethodPost {
			return
		}
		b := make([]byte, 16)
		rand.Read(b)
		secret := "hv_live_" + hex.EncodeToString(b)
		id := hex.EncodeToString(b[:8])
		mu.Lock()
		live[id] = true
		mu.Unlock()
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]string{"secret": secret, "prefix": secret[:14], "id": id, "notice": "shown once"})
		log.Printf("minted %s", id)
	})
	http.HandleFunc("/api/tokens/", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := verify(w, r); !ok || r.Method != http.MethodDelete {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/tokens/")
		mu.Lock()
		was := live[id]
		delete(live, id)
		mu.Unlock()
		if !was {
			http.Error(w, `{"error":"no such live token"}`, 404)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"revoked": id})
		log.Printf("revoked %s", id)
	})
	http.HandleFunc("/api/_live", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(map[string]int{"live": len(live)})
	})
	log.Fatal(http.ListenAndServe(*addr, nil))
}
