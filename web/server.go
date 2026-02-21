package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"time"

	helmv1alpha1 "github.com/example/helm-operator/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed static/index.html
var staticFS embed.FS

// sseClient represents one connected browser EventSource.
type sseClient struct {
	ch chan string
}

// broker fans out SSE events to all connected clients.
type broker struct {
	mu      sync.Mutex
	clients map[*sseClient]struct{}
}

func newBroker() *broker {
	return &broker{clients: make(map[*sseClient]struct{})}
}

func (b *broker) subscribe() *sseClient {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := &sseClient{ch: make(chan string, 16)}
	b.clients[c] = struct{}{}
	return c
}

func (b *broker) unsubscribe(c *sseClient) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, c)
	close(c.ch)
}

// broadcast sends a JSON payload to every connected SSE client.
// Slow clients drop the event (non-blocking send); they will re-sync on the next full list fetch.
func (b *broker) broadcast(payload string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for c := range b.clients {
		select {
		case c.ch <- payload:
		default:
		}
	}
}

// sseEvent wraps an event type and a HelmRelease resource into an SSE payload.
type sseEvent struct {
	Type     string                   `json:"type"`
	Resource *helmv1alpha1.HelmRelease `json:"resource,omitempty"`
}

// createRequest is the body expected by POST /api/helmreleases.
type createRequest struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Chart           string `json:"chart"`
	RepoURL         string `json:"repoURL"`
	Version         string `json:"version"`
	TargetNamespace string `json:"targetNamespace"`
	ReleaseName     string `json:"releaseName"`
	Values          string `json:"values"` // raw JSON string, may be empty
}

// WebServer is a controller-runtime Runnable that serves the web UI and REST API.
type WebServer struct {
	Client client.Client
	Addr   string

	broker *broker
}

// Start implements manager.Runnable.
// The manager calls this after the cache is synced and cancels ctx on shutdown.
func (s *WebServer) Start(ctx context.Context) error {
	s.broker = newBroker()

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("web: embed sub: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/helmreleases", s.handleHelmReleases)
	mux.HandleFunc("/api/events", s.handleSSE)

	srv := &http.Server{Addr: s.Addr, Handler: mux}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	ctrl.Log.Info("Starting UI server", "addr", s.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleHelmReleases routes GET/POST/PUT/DELETE for /api/helmreleases.
func (s *WebServer) handleHelmReleases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listReleases(w, r)
	case http.MethodPost:
		s.createRelease(w, r)
	case http.MethodPut:
		s.updateRelease(w, r)
	case http.MethodDelete:
		s.deleteRelease(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *WebServer) listReleases(w http.ResponseWriter, r *http.Request) {
	var list helmv1alpha1.HelmReleaseList
	if err := s.Client.List(r.Context(), &list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list.Items)
}

func (s *WebServer) createRelease(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Namespace == "" || req.Chart == "" || req.RepoURL == "" || req.Version == "" || req.TargetNamespace == "" {
		http.Error(w, "name, namespace, chart, repoURL, version, and targetNamespace are required", http.StatusBadRequest)
		return
	}

	hr := &helmv1alpha1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: helmv1alpha1.HelmReleaseSpec{
			Chart:           req.Chart,
			RepoURL:         req.RepoURL,
			Version:         req.Version,
			TargetNamespace: req.TargetNamespace,
			ReleaseName:     req.ReleaseName,
		},
	}
	if req.Values != "" {
		hr.Spec.Values = &apiextensionsv1.JSON{Raw: json.RawMessage(req.Values)}
	}

	if err := s.Client.Create(r.Context(), hr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.broadcastEvent("created", hr)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, hr)
}

func (s *WebServer) updateRelease(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	ns := r.URL.Query().Get("ns")
	if name == "" || ns == "" {
		http.Error(w, "query params 'name' and 'ns' are required", http.StatusBadRequest)
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var hr helmv1alpha1.HelmRelease
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: ns}, &hr); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	patch := client.MergeFrom(hr.DeepCopy())
	if req.Chart != "" {
		hr.Spec.Chart = req.Chart
	}
	if req.RepoURL != "" {
		hr.Spec.RepoURL = req.RepoURL
	}
	if req.Version != "" {
		hr.Spec.Version = req.Version
	}
	if req.TargetNamespace != "" {
		hr.Spec.TargetNamespace = req.TargetNamespace
	}
	hr.Spec.ReleaseName = req.ReleaseName
	if req.Values != "" {
		hr.Spec.Values = &apiextensionsv1.JSON{Raw: json.RawMessage(req.Values)}
	} else {
		hr.Spec.Values = nil
	}

	if err := s.Client.Patch(r.Context(), &hr, patch); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.broadcastEvent("updated", &hr)
	writeJSON(w, hr)
}

func (s *WebServer) deleteRelease(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	ns := r.URL.Query().Get("ns")
	if name == "" || ns == "" {
		http.Error(w, "query params 'name' and 'ns' are required", http.StatusBadRequest)
		return
	}

	hr := &helmv1alpha1.HelmRelease{}
	hr.Name = name
	hr.Namespace = ns

	if err := s.Client.Delete(r.Context(), hr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.broadcastEvent("deleted", hr)
	w.WriteHeader(http.StatusNoContent)
}

// handleSSE streams HelmRelease events to the browser via Server-Sent Events.
func (s *WebServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := s.broker.subscribe()
	defer s.broker.unsubscribe(sub)

	// Send a ping immediately so the browser knows it is connected.
	fmt.Fprintf(w, "data: {\"type\":\"ping\"}\n\n")
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case payload, ok := <-sub.ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, "data: {\"type\":\"ping\"}\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *WebServer) broadcastEvent(eventType string, hr *helmv1alpha1.HelmRelease) {
	ev := sseEvent{Type: eventType, Resource: hr}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.broker.broadcast(string(data))
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
