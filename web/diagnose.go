package web

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	helmv1alpha1 "github.com/example/helm-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *WebServer) handleDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	ns := r.URL.Query().Get("ns")
	if name == "" || ns == "" {
		http.Error(w, "query params 'name' and 'ns' are required", http.StatusBadRequest)
		return
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		http.Error(w, "ANTHROPIC_API_KEY not set", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	var hr helmv1alpha1.HelmRelease
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: ns}, &hr); err != nil {
		fmt.Fprintf(w, "data: {\"error\":%q}\n\n", err.Error())
		flusher.Flush()
		return
	}

	var events corev1.EventList
	_ = s.Client.List(r.Context(), &events, client.InNamespace(ns))

	var sb strings.Builder
	sb.WriteString("You are a Kubernetes and Helm expert. A HelmRelease has failed. Diagnose the problem and suggest a fix.\n\n")
	fmt.Fprintf(&sb, "HelmRelease: %s in namespace %s\n", name, ns)
	fmt.Fprintf(&sb, "Chart: %s %s from %s\n", hr.Spec.Chart, hr.Spec.Version, hr.Spec.RepoURL)
	fmt.Fprintf(&sb, "Phase: %s\n", hr.Status.Phase)
	sb.WriteString("\nStatus Conditions:\n")
	for _, c := range hr.Status.Conditions {
		fmt.Fprintf(&sb, "  - Type: %s, Status: %s, Reason: %s, Message: %s\n",
			c.Type, c.Status, c.Reason, c.Message)
	}
	sb.WriteString("\nRecent Kubernetes Events:\n")
	for _, ev := range events.Items {
		if ev.InvolvedObject.Name == name {
			fmt.Fprintf(&sb, "  - Reason: %s, Message: %s\n", ev.Reason, ev.Message)
		}
	}
	sb.WriteString("\nProvide a concise diagnosis (2-3 sentences) and a concrete suggested fix.")

	if err := streamDiagnosis(r.Context(), apiKey, sb.String(), w, flusher); err != nil {
		fmt.Fprintf(w, "data: {\"error\":%q}\n\n", err.Error())
		flusher.Flush()
	}
}
