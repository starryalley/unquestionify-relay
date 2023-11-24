package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Garmin connect IQ store Unquestionify app ID
const appId = "c2842d1b-ad5c-47c6-b28f-cc495abd7d32"

type notification map[int][]byte                // page -> bitmap byte array
type notificationMap map[string]notification    // notification ID -> notification
var database = make(map[string]notificationMap) // session ID -> notificationMap
var mutex = &sync.RWMutex{}

var startTime time.Time

func init() {
	startTime = time.Now()
}

func uptime() time.Duration {
	return time.Since(startTime)
}

func checkAppId(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("app-id") != appId {
		log.Println("app-id not match!")
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return false
	}
	return true
}

// URL format: /session?session=[session ID]
func serveSession(w http.ResponseWriter, r *http.Request) {
	if !checkAppId(w, r) {
		return
	}
	session := r.URL.Query().Get("session")
	mutex.Lock()
	defer mutex.Unlock()
	if r.Method == "POST" {
		if len(session) == 0 {
			log.Printf("invalid session ID: %s\n", session)
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
	} else if r.Method == "DELETE" {
		if _, ok := database[session]; !ok {
			log.Printf("session [%s] doesn't exist\n", session)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		// delete the entry from the database
		delete(database, session)
		log.Printf("[%s] ended\n", session)
	} else {
		log.Printf("session %s request not supported\n", r.Method)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// URL format: /notifications/[ID]/[page]?session=[session ID]
func serveNotification(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	mutex.RLock()
	nm, ok := database[session]
	if !ok {
		log.Printf("session [%s] doesn't exist\n", session)
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		mutex.RUnlock()
		return
	}
	mutex.RUnlock()

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
	// 16KB max payload
	r.Body = http.MaxBytesReader(w, r.Body, 16000)

	if r.Method == "PUT" {
		// uploading/updating a notification bitmap (from Android companion app)
		bitmap, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[%s][%s] %s: error: %v\n", session, r.Method, r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		if _, ok := nm[notificationId]; !ok {
			nm[notificationId] = make(notification)
		}
		nm[notificationId][page] = bitmap
		log.Printf("[%s][%s] %s: received %d bytes", session, r.Method, r.URL.Path, len(bitmap))
	} else if r.Method == "GET" {
		// downloading a notification bitmap (from the watch app)
		mutex.RLock()
		defer mutex.RUnlock()
		n, ok := nm[notificationId]
		if !ok {
			log.Printf("[%s][%s] %s: notification doesn't exist\n", session, r.Method, r.URL.Path)
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		// get notification at page p
		if _, ok := n[page]; !ok {
			log.Printf("[%s][%s] %s: page doesn't exist\n", session, r.Method, r.URL.Path)
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		// return the bitmap byte array
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprint(len(n[page])))
		count, err := w.Write(n[page])
		if err != nil {
			log.Printf("[%s][%s] %s: failed: %v\n", session, r.Method, r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		log.Printf("[%s][%s] %s: sent %d bytes\n", session, r.Method, r.URL.Path, count)
	} else if r.Method == "DELETE" {
		mutex.Lock()
		defer mutex.Unlock()
		// remove all bitmaps from a notification, page in the URL is required but not used
		if _, ok := nm[notificationId]; !ok {
			log.Printf("[%s][%s] %s: notification doesn't exist\n", session, r.Method, r.URL.Path)
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		delete(database[session], notificationId)
		log.Printf("[%s][%s] %s: done\n", session, r.Method, r.URL.Path)
	} else {
		log.Printf("[%s][%s] %s: request not supported\n", session, r.Method, r.URL.Path)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func printStat() {
	nc := 0
	bc := 0
	b := 0
	for _, nm := range database {
		nc += len(nm)
		for _, n := range nm {
			bc += len(n)
			for _, a := range n {
				b += len(a)
			}
		}
	}
	log.Printf("[Stat] Uptime:%s, Session:%d, Total notifications:%d, Total bitmap count:%d, Memory usage:%d bytes\n",
		uptime(), len(database), nc, bc, b)
}

func main() {
	httpPort := flag.Int("http-port", 80, "The port for the http service")
	httpsPort := flag.Int("https-port", 443, "The port for the https service")
	flag.Parse()

	standalone := true
	port := os.Getenv("PORT")
	if port != "" {
		standalone = false
		p, err := strconv.Atoi(port)
		if err != nil {
			log.Fatalf("Environment variable PORT isn't valid:%s", port)
		}
		*httpPort = p
		// disable timestamp in log
		log.SetFlags(0)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/session", serveSession)
	mux.HandleFunc("/notifications/", serveNotification)

	// print statistics
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		for {
			select {
			case <-ticker.C:
				printStat()
			}
		}
	}()

	// reference: https://blog.cloudflare.com/exposing-go-on-the-internet/
	cfg := &tls.Config{
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
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *httpPort),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  time.Minute,
		Handler:      mux,
	}
	httpsServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *httpsPort),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  time.Minute,
		Handler:      mux,
		TLSConfig:    cfg,
	}

	if standalone {
		log.Printf("Starting unquestionify relay server at http port %d, https port %d\n", *httpPort, *httpsPort)
		go httpServer.ListenAndServe()
		log.Fatal(httpsServer.ListenAndServeTLS("fullchain.pem", "privkey.pem"))
	} else {
		log.Printf("Starting unquestionify relay server at http port %d", *httpPort)
		httpServer.ListenAndServe()
	}

}
