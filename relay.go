package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

const appId = "c2842d1b-ad5c-47c6-b28f-cc495abd7d32"

type notification [][]byte                      // page as array index -> bitmap byte array
type notificationMap map[string]notification    // notification ID -> notification
var database = make(map[string]notificationMap) // session ID -> notificationMap

func checkAppId(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("app-id") != appId {
		log.Println("app-id not match!")
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return false
	}
	return true
}

func sessionStart(w http.ResponseWriter, r *http.Request) {
	if !checkAppId(w, r) {
		return
	}
	if r.Method != "POST" {
		log.Printf("session %s request not supported\n", r.Method)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	session := r.URL.Query().Get("session")
	if len(session) == 0 {
		log.Printf("invalid session: %s\n", session)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if _, ok := database[session]; ok {
		log.Printf("session [%s] already exists\n", session)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	// create a new entry in the database
	database[session] = make(notificationMap)
	log.Printf("[%s] started\n", session)
}

func sessionEnd(w http.ResponseWriter, r *http.Request) {
	if !checkAppId(w, r) {
		return
	}
	if r.Method != "POST" {
		log.Printf("session %s request not supported\n", r.Method)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	session := r.URL.Query().Get("session")
	if _, ok := database[session]; !ok {
		log.Printf("session [%s] doesn't exist\n", session)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	// delete the entry from the database
	delete(database, session)
	log.Printf("[%s] ended\n", session)
}

// URL format: /notifications/[ID]/[page]
func serveNotification(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("session")
	nm, ok := database[session]
	if !ok {
		log.Printf("session [%s] doesn't exist\n", session)
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	urlParts := strings.Split(r.URL.Path, "/")
	if len(urlParts) != 4 {
		log.Printf("[%s][%s] %s: URL invalid", session, r.Method, r.URL.Path)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	notificationId := urlParts[2]
	p := urlParts[3]
	page, err := strconv.Atoi(p)
	if err != nil {
		log.Printf("[%s][%s] %s: page invalid\n", session, r.Method, r.URL.Path)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if r.Method == "PUT" {
		// uploading a notification bitmap (from Android companion app)
		bitmap, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[%s][%s] %s: error: %v\n", session, r.Method, r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		n, ok := nm[notificationId]
		if !ok {
			nm[notificationId] = make(notification, 1)
			// page should be 0. If not, this is a violation
			if page != 0 {
				log.Printf("[%s][%s] %s: expecting page = 0", session, r.Method, r.URL.Path)
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			nm[notificationId][0] = bitmap
		} else {
			// page should at most len(n). If not, this is a violation too
			if page > len(n) {
				log.Printf("[%s][%s] %s: expecting page max %d", session, r.Method, r.URL.Path, len(n))
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			} else if page == len(n) {
				nm[notificationId] = append(nm[notificationId], bitmap)
			} else {
				nm[notificationId][page] = bitmap
			}
		}
		log.Printf("[%s][%s] %s: received %d bytes", session, r.Method, r.URL.Path, len(bitmap))
	} else if r.Method == "GET" {
		// downloading a notification bitmap (from the watch app)
		n, ok := nm[notificationId]
		if !ok {
			log.Printf("[%s][%s] %s: notification doesn't exist\n", session, r.Method, r.URL.Path)
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		// get notification at page p
		if page >= len(n) {
			log.Printf("[%s][%s] %s: page doesn't exist\n", session, r.Method, r.URL.Path)
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		// return the bitmap byte array
		w.Header().Set("Content=Type", "image/png")
		count, err := w.Write(n[page])
		if err != nil {
			log.Printf("[%s][%s] %s: failed: %v\n", session, r.Method, r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		log.Printf("[%s][%s] %s: sent %d bytes\n", session, r.Method, r.URL.Path, count)
	} else {
		log.Printf("[%s][%s] %s: request not supported\n", session, r.Method, r.URL.Path)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func main() {
	https := flag.Bool("use-https", false, "Use HTTPS instead of HTTP")
	port := flag.Int("port", 8080, "The port for the https service")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/start_session", sessionStart)
	mux.HandleFunc("/end_session", sessionEnd)
	mux.HandleFunc("/notifications/", serveNotification)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  time.Minute,
		Handler:      mux,
	}

	if *https {
		// Configure autocert settings
		// https://gist.github.com/alexedwards/fd7e2725962be79a00818488ea1bcd00
		autocertManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist("https://applepinemango.duckdns.org/"),
			Cache:      autocert.DirCache("cert"),
		}

		// reference: https://blog.cloudflare.com/exposing-go-on-the-internet/
		cfg := &tls.Config{
			GetCertificate:           autocertManager.GetCertificate,
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP256, tls.X25519},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		}
		srv.TLSConfig = cfg
		log.Printf("Starting HTTPS unquestionify relay at port %d\n", *port)
		log.Fatal(srv.ListenAndServeTLS("", ""))
	} else {
		log.Printf("Starting HTTP unquestionify relay at port %d\n", *port)
		log.Fatal(srv.ListenAndServe())
	}

}
