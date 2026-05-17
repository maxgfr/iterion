package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

func TestInit_noEndpoint_isNoOp(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	shutdown, err := Init(context.Background(), "iterion-test", nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init must always return a non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestParseRatio(t *testing.T) {
	cases := []struct {
		in       string
		fallback float64
		want     float64
	}{
		{"", 0.5, 0.5},
		{"0.1", 1, 0.1},
		{"1", 1, 1},
		{"2", 1, 1},
		{"-0.5", 1, 0},
		{"not-a-float", 0.42, 0.42},
	}
	for _, tc := range cases {
		if got := parseRatio(tc.in, tc.fallback); got != tc.want {
			t.Errorf("parseRatio(%q, %v) = %v, want %v", tc.in, tc.fallback, got, tc.want)
		}
	}
}

func TestEnvSampler_default(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "")
	if s := envSampler(); s == nil {
		t.Fatal("envSampler must always return a sampler")
	}
}

func TestEnvSampler_alwaysOff(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")
	if got, want := envSampler().Description(), tracesdk.NeverSample().Description(); got != want {
		t.Errorf("Description = %q, want %q", got, want)
	}
}

func TestEnvSampler_ratio(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
	s := envSampler()
	if s == nil {
		t.Fatal("nil sampler")
	}
	// Description carries the ratio so the operator can confirm the
	// config landed.
	if got := s.Description(); got == "" {
		t.Error("expected non-empty sampler description")
	}
}

func TestEnvSampler_unknown_fallsBackToParentBasedAlwaysOn(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "no-such-sampler")
	s := envSampler()
	want := tracesdk.ParentBased(tracesdk.AlwaysSample()).Description()
	if got := s.Description(); got != want {
		t.Errorf("unknown sampler: Description = %q, want %q", got, want)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "third"); got != "third" {
		t.Errorf("firstNonEmpty = %q, want third", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty(empties) = %q, want empty", got)
	}
}

func TestInit_withFakeEndpoint_buildsTracerProvider(t *testing.T) {
	// Fake OTLP collector that accepts everything. The exporter is
	// lazy on the wire so Init() succeeds even when the collector
	// never receives a span before shutdown.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", srv.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := Init(context.Background(), "iterion-test", nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init must return a non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestInit_endpointWithoutScheme_buildsTracerProvider(t *testing.T) {
	// "host:port" form takes the WithEndpoint branch, not WithEndpointURL.
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "localhost:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := Init(context.Background(), "iterion-test", nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}
