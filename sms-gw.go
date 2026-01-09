  // sms-gateway-daemon/main.go
  // A tiny SMS gateway daemon that accepts HTTP and/or MQTT requests and
// sends messages via an AT-compatible modem.
//
// Features
// - One-time modem init (CPIN?, optional PIN, CMGF? -> 1, CSCS? -> GSM)
// - HTTP POST /sms { "to": "+3069...", "message": "..." }
// - MQTT topic: <MQTT_TOPIC> payload JSON {to,message}
// - Work queue with retries, jitter, and rate-limiting
// - Graceful shutdown
// - Basic token auth for HTTP (optional)
//
// Env config
//  PORT=/dev/ttyUSB2
//  BAUD=115200
//  SIM_PIN=1980                # optional
//  HTTP_ADDR=:8080             # optional, empty disables HTTP
//  HTTP_TOKEN=secret           # optional, if set is required in Authorization: Bearer <token>
//  MQTT_BROKER=tcp://localhost:1883   # optional, empty disables MQTT
//  MQTT_CLIENT_ID=sms-gw-1
//  MQTT_TOPIC=sms/send
//  MQTT_USERNAME=...
//  MQTT_PASSWORD=...
//  RATE_PER_MIN=30             # basic rate limiting
//  MAX_RETRIES=3
//
// Build: go build -o sms-gw
// Run:   ./sms-gw
// Docker: see Dockerfile in this repo.

package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	serial "github.com/tarm/serial"
)

// ---------- Models ----------

type SmsReq struct {
	To      string `json:"to"`
	Message string `json:"message"`
	ID      string `json:"id,omitempty"` // optional caller-supplied id
}

// ---------- Config ----------

type Config struct {
	Port         string
	Baud         int
	SimPIN       string
	HttpAddr     string
	HttpToken    string
	MqttBroker   string
	MqttClientID string
	MqttTopic    string
	MqttUser     string
	MqttPass     string
	RatePerMin   int
	MaxRetries   int
	EchoOn       bool // ATE1 if true, ATE0 if false
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" { return def }
	return v
}

func mustAtoi(s string, def int) int {
	if s == "" { return def }
	var x int
	_, err := fmt.Sscanf(s, "%d", &x)
	if err != nil { return def }
	return x
}

func loadConfig() Config {
	// ECHO: "1" enables ATE1, anything else ATE0
	echo := strings.TrimSpace(os.Getenv("ECHO")) == "1"
	return Config{
		Port:         getenv("PORT", "/dev/ttyUSB2"),
		Baud:         mustAtoi(getenv("BAUD", "115200"), 115200),
		SimPIN:       getenv("SIM_PIN", ""),
		HttpAddr:     getenv("HTTP_ADDR", ":8080"),
		HttpToken:    getenv("HTTP_TOKEN", ""),
		MqttBroker:   getenv("MQTT_BROKER", ""),
		MqttClientID: getenv("MQTT_CLIENT_ID", "sms-gw-1"),
		MqttTopic:    getenv("MQTT_TOPIC", "sms/send"),
		MqttUser:     getenv("MQTT_USERNAME", ""),
		MqttPass:     getenv("MQTT_PASSWORD", ""),
		RatePerMin:   mustAtoi(getenv("RATE_PER_MIN", "30"), 30),
		MaxRetries:   mustAtoi(getenv("MAX_RETRIES", "3"), 3),
		EchoOn:       echo,
	}
}

// ---------- Modem ----------

type Modem struct {
	mu   sync.Mutex
	ser  *serial.Port
	cfg  Config
}

func NewModem(cfg Config) (*Modem, error) {
	c := &serial.Config{Name: cfg.Port, Baud: cfg.Baud, ReadTimeout: 200 * time.Millisecond}
	p, err := serial.OpenPort(c)
	if err != nil { return nil, err }
	m := &Modem{ser: p, cfg: cfg}
	if err := m.handshake(); err != nil { return nil, err }
	if err := m.ensureReady(); err != nil { return nil, err }
	return m, nil
}

func (m *Modem) write(s string) error {
	_, err := m.ser.Write([]byte(s))
	return err
}

func (m *Modem) readLines(deadline time.Time) []string {
	var out []string
	buf := make([]byte, 256)
	line := ""
	for time.Now().Before(deadline) {
		n, _ := m.ser.Read(buf)
		if n == 0 { continue }
		for _, b := range buf[:n] {
			if b == '\r' { continue }
			if b == '\n' { if strings.TrimSpace(line) != "" { out = append(out, strings.TrimSpace(line)) }; line = ""; continue }
			line += string(b)
		}
	}
	if strings.TrimSpace(line) != "" { out = append(out, strings.TrimSpace(line)) }
	return out
}

func (m *Modem) send(cmd string, expectPrompt bool, timeout time.Duration) ([]string, bool, bool) {
	_ = m.write(cmd + "\r")
	dl := time.Now().Add(timeout)
	lines := m.readLines(dl)
	ok := false
	prompt := false
	for _, l := range lines {
		if l == ">" { prompt = true }
		if l == "OK" { ok = true }
	}
	if expectPrompt && prompt { return lines, ok, true }
	return lines, ok, prompt
}

func (m *Modem) handshake() error {
	for i := 0; i < 4; i++ {
		_, ok, _ := m.send("AT", false, 800*time.Millisecond)
		if ok {
			// set echo state according to config (default ATE0 for clean parsing)
			if m.cfg.EchoOn {
				m.send("ATE1", false, 1*time.Second)
			} else {
				m.send("ATE0", false, 1*time.Second)
			}
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("modem not responding to AT")
}

func (m *Modem) ensureReady() error {
	m.mu.Lock(); defer m.mu.Unlock()
	// CPIN?
	lines, _, _ := m.send("AT+CPIN?", false, 2*time.Second)
	state := strings.Join(lines, " ")
	if strings.Contains(state, "+CPIN: SIM PIN") {
		if m.cfg.SimPIN == "" { return errors.New("SIM requires PIN; set SIM_PIN env") }
		if _, ok, _ := m.send("AT+CPIN="+m.cfg.SimPIN, false, 5*time.Second); !ok {
			return errors.New("failed to submit SIM PIN")
		}
		time.Sleep(2 * time.Second)
	}
	// CMGF?
	lines, _, _ = m.send("AT+CMGF?", false, 2*time.Second)
	joined := strings.Join(lines, " ")
	if !strings.Contains(joined, "+CMGF: 1") {
		if _, ok, _ := m.send("AT+CMGF=1", false, 3*time.Second); !ok {
			return errors.New("failed to set text mode")
		}
	}
	// CSCS? -> ensure GSM
	lines, _, _ = m.send("AT+CSCS?", false, 2*time.Second)
	if !strings.Contains(strings.Join(lines, " "), "\"GSM\"") {
		_, _ok, _ := m.send("AT+CSCS=\"GSM\"", false, 3*time.Second)
		if !_ok { log.Printf("warn: could not set CSCS=GSM; continuing") }
	}
	return nil
}

func (m *Modem) SendSMS(to, body string) (string, error) {
	m.mu.Lock(); defer m.mu.Unlock()
	if _, ok, _ := m.send(fmt.Sprintf("AT+CMGS=\"%s\"", to), true, 7*time.Second); !ok {
		// even without OK, if we saw a prompt we'll continue
	}
	// write message + Ctrl+Z
	if err := m.write(body); err != nil { return "", err }
	if err := m.write(string([]byte{26})); err != nil { return "", err }
	lines := m.readLines(time.Now().Add(25 * time.Second))
	var ref string
	var gotOK bool
	for _, l := range lines {
		if strings.HasPrefix(l, "+CMGS:") { ref = strings.TrimSpace(strings.TrimPrefix(l, "+CMGS:")) }
		if l == "OK" { gotOK = true }
	}
	if !gotOK { return ref, fmt.Errorf("no OK after CMGS; lines=%v", lines) }
	return ref, nil
}

// ---------- Rate limiter ----------

type Rate struct {
	mu sync.Mutex
	cap int
	win []time.Time
}

func NewRate(nPerMin int) *Rate { return &Rate{cap: nPerMin} }

func (r *Rate) Allow() bool {
	r.mu.Lock(); defer r.mu.Unlock()
	now := time.Now()
	cut := now.Add(-1 * time.Minute)
	var kept []time.Time
	for _, t := range r.win { if t.After(cut) { kept = append(kept, t) } }
	r.win = kept
	if len(r.win) >= r.cap { return false }
	r.win = append(r.win, now)
	return true
}

// ---------- Worker ----------

type Job struct { Req SmsReq; Attempts int }

type Gateway struct {
	cfg   Config
	m     *Modem
	q     chan Job
	limit *Rate
}

func NewGateway(cfg Config, m *Modem) *Gateway {
	g := &Gateway{cfg: cfg, m: m, q: make(chan Job, 1024), limit: NewRate(cfg.RatePerMin)}
	return g
}

func (g *Gateway) Enqueue(r SmsReq) {
	if r.ID == "" {
		h := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%d", r.To, r.Message, time.Now().UnixNano())))
		r.ID = hex.EncodeToString(h[:8])
	}
	g.q <- Job{Req: r}
}

func (g *Gateway) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-g.q:
			if !g.limit.Allow() {
				// backoff 2s and retry
				time.Sleep(2 * time.Second)
				job.Attempts++
				g.q <- job
				continue
			}
			ref, err := g.m.SendSMS(job.Req.To, job.Req.Message)
			if err != nil {
				if job.Attempts < g.cfg.MaxRetries {
					back := time.Duration(800+rand.Intn(600)) * time.Millisecond
					log.Printf("send fail id=%s err=%v; retrying in %v", job.Req.ID, err, back)
					time.Sleep(back)
					job.Attempts++
					g.q <- job
					continue
				}
				log.Printf("send permanent fail id=%s to=%s err=%v", job.Req.ID, job.Req.To, err)
				continue
			}
			log.Printf("send ok id=%s to=%s ref=%s", job.Req.ID, job.Req.To, strings.TrimSpace(ref))
		}
	}
}

// ---------- HTTP ----------

func (g *Gateway) startHTTP(ctx context.Context) *http.Server {
	if g.cfg.HttpAddr == "" { return nil }
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/sms", func(w http.ResponseWriter, r *http.Request) {
		if g.cfg.HttpToken != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != g.cfg.HttpToken {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("unauthorized"))
				return
			}
		}
		if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
		var req SmsReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil { w.WriteHeader(400); _, _ = w.Write([]byte("bad json")); return }
		if req.To == "" || req.Message == "" { w.WriteHeader(400); _, _ = w.Write([]byte("missing to/message")); return }
		g.Enqueue(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status":"queued","id":req.ID})
	})
	srv := &http.Server{Addr: g.cfg.HttpAddr, Handler: mux}
	go func(){
		log.Printf("http listening on %s", g.cfg.HttpAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server error: %v", err)
		}
	}()
	go func(){ <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	return srv
}

// ---------- MQTT ----------

func (g *Gateway) startMQTT(ctx context.Context) mqtt.Client {
	if g.cfg.MqttBroker == "" { return nil }
	opts := mqtt.NewClientOptions()
	opts.AddBroker(g.cfg.MqttBroker)
	opts.SetClientID(g.cfg.MqttClientID)
	if g.cfg.MqttUser != "" { opts.SetUsername(g.cfg.MqttUser); opts.SetPassword(g.cfg.MqttPass) }
	opts.SetOrderMatters(false)
	opts.SetAutoReconnect(true)
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error){ log.Printf("mqtt lost: %v", err) })
	opts.SetOnConnectHandler(func(c mqtt.Client){
		log.Printf("mqtt connected, subscribing %s", g.cfg.MqttTopic)
		if token := c.Subscribe(g.cfg.MqttTopic, 0, func(_ mqtt.Client, m mqtt.Message){
			var req SmsReq
			if err := json.Unmarshal(m.Payload(), &req); err != nil {
				log.Printf("mqtt bad payload: %v", err); return
			}
			if req.To == "" || req.Message == "" { log.Printf("mqtt missing to/message"); return }
			g.Enqueue(req)
		}); token.Wait() && token.Error() != nil {
			log.Printf("mqtt subscribe error: %v", token.Error())
		}
	})
	cli := mqtt.NewClient(opts)
	t := cli.Connect(); t.Wait()
	if t.Error() != nil { log.Printf("mqtt connect error: %v", t.Error()) }
	go func(){ <-ctx.Done(); if cli != nil { cli.Disconnect(500) } }()
	return cli
}

// ---------- main ----------

func main() {
	rand.Seed(time.Now().UnixNano())
	cfg := loadConfig()
	log.Printf("starting sms-gateway on %s (HTTP=%v MQTT=%v)", cfg.Port, cfg.HttpAddr != "", cfg.MqttBroker != "")

	m, err := NewModem(cfg)
	if err != nil { log.Fatalf("modem init failed: %v", err) }
	gw := NewGateway(cfg, m)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go gw.worker(ctx)
	_ = gw.startHTTP(ctx)
	_ = gw.startMQTT(ctx)

	<-ctx.Done()
	log.Printf("shutting down")
}

