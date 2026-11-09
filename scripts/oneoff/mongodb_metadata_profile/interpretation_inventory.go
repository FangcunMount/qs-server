package main

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"go.mongodb.org/mongo-driver/bson"
)

func runLegacyFutureInventory(w io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	out := struct {
		Source         string                                `json:"source_sha"`
		ObservedAt     string                                `json:"observed_at"`
		Owner          string                                `json:"owner"`
		Binding        string                                `json:"database_replica_sha256,omitempty"`
		ReadOnly       bool                                  `json:"read_only"`
		Complete       bool                                  `json:"inventory_complete"`
		DropAuthorized bool                                  `json:"drop_authorized"`
		PoolReleased   bool                                  `json:"pool_released"`
		Counts         *interpretation.LegacyFutureInventory `json:"counts,omitempty"`
		Error          string                                `json:"error_phase"`
	}{Source: retirementSourceSHA, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Owner: "qs.mongo", ReadOnly: true, Error: "application_binding"}
	if !shaRE.MatchString(retirementSourceSHA) {
		return 1
	}
	if c, err := retirementConnection(ctx); err == nil {
		out.Error = "replica_identity"
		hello, err := c.Command(ctx, "admin", bson.D{{Key: "hello", Value: 1}})
		set, setOK := hello.Lookup("setName").StringValueOK()
		primary, primaryOK := hello.Lookup("isWritablePrimary").BooleanOK()
		if err == nil && setOK && set != "" && primaryOK && primary {
			out.Binding = retirementHash([]byte("qs\x00" + set))
			out.Error = "execution_inventory"
			counts, err := interpretation.InventoryLegacyFutures(ctx, c.client.Database("qs"))
			if err == nil {
				out.Counts = &counts
				out.Error = "none"
			}
		}
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		out.PoolReleased = c.Close(closeCtx) == nil
		closeCancel()
		if !out.PoolReleased {
			out.Counts = nil
			out.Error = "release"
		}
	}
	out.Complete = out.Counts != nil
	if json.NewEncoder(w).Encode(out) != nil || !out.Complete {
		return 1
	}
	return 0
}
