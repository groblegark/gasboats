package server

import (
	"encoding/json"
	"testing"

	beadsv1 "github.com/groblegark/kbeads/gen/beads/v1"
	"github.com/groblegark/kbeads/internal/model"
)

func TestGRPCSetConfig(t *testing.T) {
	srv, _, ctx := testCtx(t)
	resp, err := srv.SetConfig(ctx, &beadsv1.SetConfigRequest{
		Key: "view:inbox", Value: []byte(`{"filter":{"status":["open"]}}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Config.Key != "view:inbox" {
		t.Fatalf("got key=%q", resp.Config.Key)
	}
}

func TestGRPCGetConfig(t *testing.T) {
	srv, ms, ctx := testCtx(t)
	ms.configs["view:inbox"] = &model.Config{Key: "view:inbox", Value: json.RawMessage(`{"filter":{"status":["open"]}}`)}

	resp, err := srv.GetConfig(ctx, &beadsv1.GetConfigRequest{Key: "view:inbox"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Config.Key != "view:inbox" {
		t.Fatalf("got key=%q", resp.Config.Key)
	}
}

func TestGRPCGetConfig_BuiltinFallback(t *testing.T) {
	srv, _, ctx := testCtx(t)
	resp, err := srv.GetConfig(ctx, &beadsv1.GetConfigRequest{Key: "view:ready"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Config.Key != "view:ready" {
		t.Fatalf("got key=%q", resp.Config.Key)
	}
}

func TestGRPCListConfigs(t *testing.T) {
	srv, ms, ctx := testCtx(t)
	ms.configs["view:inbox"] = &model.Config{Key: "view:inbox", Value: json.RawMessage(`{}`)}

	resp, err := srv.ListConfigs(ctx, &beadsv1.ListConfigsRequest{Namespace: "view"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 stored + 1 builtin (view:ready)
	if len(resp.Configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(resp.Configs))
	}
}

func TestGRPCDeleteConfig(t *testing.T) {
	srv, ms, ctx := testCtx(t)
	ms.configs["view:inbox"] = &model.Config{Key: "view:inbox", Value: json.RawMessage(`{}`)}

	if _, err := srv.DeleteConfig(ctx, &beadsv1.DeleteConfigRequest{Key: "view:inbox"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := ms.configs["view:inbox"]; ok {
		t.Fatal("expected config to be deleted")
	}
}
